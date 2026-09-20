# openjev

Local Decision Model prototype. A Go HTTP API and CLI call a pinned
[llama.cpp](https://github.com/ggml-org/llama.cpp) `llama-server` (CUDA) with
MiniCPM5-2B GGUF, and answer questions in the style of
[Jev / System One](https://www.jevai.org/docs): **choice**, **score**, and
**noul**.

The decision path can read first-token option probabilities (`direct`), run a
constrained JSON generation (`generation`), or both. A `load` command fans out
concurrent requests for throughput measurement.

Primary code lives under `features/decision-test`. Paths, ports, and model pins
are in `settings/decision-test.yaml`. Model weights and llama.cpp binaries are
not committed (`models/`, `third_party/`, `bin/`).

## Requirements

- Windows x64 with an NVIDIA GPU and a working CUDA stack (this repo pins the
  llama.cpp **CUDA 12.4** zip)
- [Go](https://go.dev/dl/) **1.24+**
- Git Bash / MSYS2 (or another bash that can run `scripts/**/*.sh`)
- Network access once, to download the GGUF and the llama.cpp release zips

VRAM note: MiniCPM5-2B Q4_K_M with `-c 2048` fits an **8 GB** laptop GPU
(RTX 3070 class). Keep `llama_parallel` / `--parallel` modest unless you have
remeasured.

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
| `llama_parallel` | `1` | Max concurrent inference calls from the API |
| `model_path` / `llama_binary` | under `models/` / `third_party/` | On-disk artifacts |

If another process already owns port `18080`, change `llama_port` and
`llama_url` together (and keep them consistent).

## Run

Use **two terminals**. llama-server stays in the foreground.

### 1. Start llama-server

```bash
./scripts/setup/run_llama_server.sh
# optional: more slots (must match llama_parallel in the API settings)
# ./scripts/setup/run_llama_server.sh --parallel 5
```

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

Flags:

- `--server` — API base URL (default: host/port from settings)
- `--method` — override `options.method` (`direct` | `generation` | `both`)
- `--json` — print the response body

### 4. Load (concurrent requests)

```bash
./bin/decision-test.exe load \
  --input features/decision-test/testdata/account.json \
  --method direct \
  --concurrency 20 \
  --slots 5 \
  --json
```

`--slots` is a **report label** only; it does not reconfigure llama-server.
Align `--concurrency` with how hard you want to push the API, and set
`run_llama_server.sh --parallel` / `llama_parallel` to the same slot count when
comparing slot sizes. On this hardware, roughly **slots 5 / concurrency ~20**
was a strong throughput vs latency tradeoff for `account.json` direct.

## Tests

```bash
# Unit tests + binary
./scripts/process/build.sh

# Integration (needs a warm llama-server matching settings/llama_url)
./scripts/process/integration_test.sh --specify "TestDecisionSystemOne"
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
