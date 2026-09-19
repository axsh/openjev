package decision

import (
	"math"
	"testing"
)

func TestSoftmaxUniform(t *testing.T) {
	got := Softmax([]float64{0, 0})
	if math.Abs(got[0]-0.5) > 1e-12 || math.Abs(got[1]-0.5) > 1e-12 {
		t.Fatalf("%v", got)
	}
}

func TestSoftmaxLarge(t *testing.T) {
	got := Softmax([]float64{1000, 0})
	if math.IsInf(got[0], 0) || got[0] < 0.999 {
		t.Fatalf("%v", got)
	}
}

func TestSoftmaxKnownRatio(t *testing.T) {
	got := Softmax([]float64{math.Log(0.25), math.Log(0.25), math.Log(0.5)})
	sum := got[0] + got[1] + got[2]
	if math.Abs(sum-1) > 1e-12 {
		t.Fatalf("sum %v", sum)
	}
	if math.Abs(got[2]/got[0]-2) > 1e-9 {
		t.Fatalf("ratio %v", got)
	}
}

func TestConfidence(t *testing.T) {
	if math.Abs(Confidence([]float64{1.0 / 3, 1.0 / 3, 1.0 / 3})) > 1e-12 {
		t.Fatalf("uniform %v", Confidence([]float64{1.0 / 3, 1.0 / 3, 1.0 / 3}))
	}
	if math.Abs(Confidence([]float64{1, 0, 0})-1) > 1e-12 {
		t.Fatalf("peaked %v", Confidence([]float64{1, 0, 0}))
	}
	if math.Abs(Confidence([]float64{0.5, 0.5})) > 1e-12 {
		t.Fatalf("two %v", Confidence([]float64{0.5, 0.5}))
	}
}

func TestPickLogprobs(t *testing.T) {
	top := []TokenLogprob{{ID: 10, Logprob: -0.1}, {ID: 11, Logprob: -0.2}, {ID: 12, Logprob: -0.3}}
	labels := []Label{{Letter: "A", TokenID: 10}, {Letter: "B", TokenID: 11}, {Letter: "C", TokenID: 12}}
	got, err := PickLogprobs(top, labels)
	if err != nil || got[2] != -0.3 {
		t.Fatalf("%v %v", got, err)
	}
	_, err = PickLogprobs(top[:2], labels)
	if err == nil || !contains(err.Error(), "C") {
		t.Fatalf("err %v", err)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || (len(s) > 0 && (s[0:len(sub)] == sub || contains(s[1:], sub))))
}
