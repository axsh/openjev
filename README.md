# openjev

Local Decision Model prototype. A Go HTTP API and CLI call a pinned
[llama.cpp](https://github.com/ggml-org/llama.cpp) `llama-server` (CUDA) with
MiniCPM5-2B GGUF, and answer questions in the style of
[Jev / System One](https://www.jevai.org/docs): **choice**, **score**, and
**noul**.

The decision path can read first-token option probabilities (`direct`), run a
constrained JSON generation (`generation`), or both. One request carries a
shared `state` and any number of `questions`; the server answers them in
parallel through a worker pool. A `load` command measures both questions per
request and concurrent requests.

Primary code lives under `features/decision-test`. Paths, ports, and model pins
are in `settings/decision-test.yaml`. Model weights and llama.cpp binaries are
not committed (`models/`, `third_party/`, `bin/`).

## Official Jev probe

`features/jev-test` sends `features/decision-test/testdata/bank.json` once to
`POST https://api.typesafe.ai/v1/systemone` and prints `wall_ms` plus the
shape of each answer. The API key is read from `tmp/typesafe-api-key.txt`,
which is gitignored. Do not commit the key.

From the repository root on Windows:

```bash
./bin/jev-test.exe
./bin/jev-test.exe --json
```

`./scripts/process/build.sh` builds `bin/jev-test.exe` and runs the unit tests.
Those tests, and `scripts/process/integration_test.sh --specify "TestJevProbe_"`,
talk to a local test server. They do not call the official endpoint.

## Requirements

- Windows x64 with an NVIDIA GPU and a working CUDA stack (this repo pins the
  llama.cpp **CUDA 12.4** zip)
- [Go](https://go.dev/dl/) **1.24+**
- Git Bash / MSYS2 (or another bash that can run `scripts/**/*.sh`)
- Network access once, to download the GGUF and the llama.cpp release zips

VRAM note: MiniCPM5-2B Q4_K_M with `-c 2048` fits an **8 GB** laptop GPU
(RTX 3070 class). Keep `--parallel` modest unless you have remeasured; slot
counts that do not divide 2048 (3, 5, 6) may pad the per-slot context and use
more VRAM.

## Environment setup

From the repository root:

```bash
# 1. Install the pinned llama-server (b11056, win-cuda-12.4) and CUDA runtime
./scripts/setup/install_llama_cpp.sh

# 2. Download MiniCPM5-2B-Q4_K_M.gguf (revision-pinned URL in settings)
./scripts/setup/download_model.sh

# 3. Build the API / CLI binary (runs unit tests, writes bin/decision-test.exe)
./scripts/process/build.sh
```

Confirm settings in `settings/decision-test.yaml`:

| Key | Default (committed) | Role |
| --- | --- | --- |
| `api_host` / `api_port` | `127.0.0.1` / `8090` | Decision API listen address |
| `llama_host` / `llama_port` | `127.0.0.1` / `18080` | llama-server listen address |
| `llama_url` | `http://127.0.0.1:18080` | URL the API uses to reach llama |
| `workers` | `16` | Worker goroutines that answer questions; also the cap on concurrent llama calls |
| `stall_timeout_ms` | `15000` | Per-request stall cutoff; missing answers are omitted from a 200 response |
| `model_path` / `llama_binary` | under `models/` / `third_party/` | On-disk artifacts |

If another process already owns port `18080`, change `llama_port` and
`llama_url` together (and keep them consistent).

## Run

Use **two terminals**. llama-server stays in the foreground.

### 1. Start llama-server

```bash
./scripts/setup/run_llama_server.sh
# optional: more slots so llama batches more questions per decode step
# ./scripts/setup/run_llama_server.sh --parallel 5
```

`--parallel` does not have to match `workers`. Workers beyond the slot count
wait inside llama-server's queue; the API never rejects on load.

Wait until `http://127.0.0.1:18080/health` returns success.

### 2. Start the decision API

```bash
./scripts/process/build.sh   # if the binary is missing or code changed
./bin/decision-test.exe
```

Health check:

```bash
curl http://127.0.0.1:8090/health
```

`ready` should be `true` after label resolve and warmup succeed.

### 3. Decide (single request)

```bash
./bin/decision-test.exe decide \
  --input features/decision-test/testdata/account.json \
  --method direct \
  --json
```

Useful fixtures under `features/decision-test/testdata/`:

- `account.json` — choice
- `score.json` — ordered score levels
- `noul.json` / `noul-omit.json` — yes/no style noul
- `mixed.json` — choice + score + noul with `method: both`
- `email.json` — generation-oriented sample
- `bank.json` — 30 questions (choice / score / noul interleaved) on one state
- `playground.json` — the Jev Playground example (3 questions)

Flags:

- `--server` — API base URL (default: host/port from settings)
- `--method` — override `options.method` (`direct` | `generation` | `both`)
- `--json` — print the response body

`instructions` may be a string, an object, or an array (objects and arrays are
embedded as JSON text, like `state`).

### 4. Load (questions per request, concurrent requests)

Questions of one request are answered in parallel by the worker pool. To
measure that, send one request at a time and vary the question count:

```bash
./bin/decision-test.exe load \
  --input features/decision-test/testdata/bank.json \
  --questions 10 \
  --method direct \
  --concurrency 1 \
  --slots 5 \
  --workers 16 \
  --json
```

To measure concurrent requests instead, raise `--concurrency`:

```bash
./bin/decision-test.exe load \
  --input features/decision-test/testdata/account.json \
  --method direct \
  --concurrency 20 \
  --slots 5 \
  --workers 16 \
  --json
```

- `--questions N` keeps only the first N questions of the input (0 = all).
- `--slots` and `--workers` are **report labels** only; they do not
  reconfigure llama-server or the API. Set `run_llama_server.sh --parallel`
  and `workers` in the settings to match what you record.
- The report adds `questions` (per request), `answers`, `answers_missing`
  (`success x questions - answers`), and `questions_per_sec`. A non-zero
  `answers_missing` makes the command exit non-zero.
- `direct_ms` (and `direct_ms_sum`) is the time of the engine call. When
  `workers` exceeds the slot count it includes the wait inside llama-server's
  queue, not only inference.

### 5. Partial answers

Each handler watches its own progress: the number of tasks it has queued and
the number of answers it has received. If neither changes for
`stall_timeout_ms`, the handler stops waiting and returns HTTP 200 with the
answers collected so far; the missing question IDs are simply absent from
`answers`, and the server logs `systemone stalled` at WARN. This also happens
to a single-question request that waits longer than `stall_timeout_ms` in the
FIFO queue under heavy concurrent load, so raise the setting if you push
`--concurrency` far past what `workers` and the slot count can drain in time.

## Tests

```bash
# Unit tests + binary
./scripts/process/build.sh

# Integration (needs a warm llama-server matching settings/llama_url)
./scripts/process/integration_test.sh --specify "TestDecisionSystemOne"

# Questions-per-request benchmark: slots 1..6 x questions 1,5,10,15,20,30 with
# 30 workers. Starts its own llama-server; stop other llama processes first.
./scripts/process/integration_test.sh --specify "TestDecisionSystemOne_Batch"
```

Integration tests that start their own llama use ports `18280` / `18199` and do
not stop a server on `18080` / `18081`.

## Layout

```
features/decision-test/   Decision API, CLI, domain, llama client
settings/                 Runtime YAML (committed defaults)
scripts/setup/            Install llama.cpp, download model, run server
scripts/process/          build.sh, integration_test.sh
tests/                    Go integration tests (tag: integration)
prompts/phases/           Specs and implementation plans (Japanese)
```

## License / upstream pins

- llama.cpp build: **b11056** (`llama-b11056-bin-win-cuda-12.4-x64.zip`)
- Model: [openbmb/MiniCPM5-2B-GGUF](https://huggingface.co/openbmb/MiniCPM5-2B-GGUF)
  file `MiniCPM5-2B-Q4_K_M.gguf` at revision
  `2079a22f3beaa4e306449978533478fe0522f4b3`
