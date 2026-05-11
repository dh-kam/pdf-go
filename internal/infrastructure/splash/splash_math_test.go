package splash

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// oracleDir is where the pinned CSV oracles live. Regen is gated by
// SPLASH_ORACLE_REGEN=1 so an accidental "improvement" of a helper still
// fails the parity test rather than silently rewriting the pin.
const oracleDir = "testdata/oracle"

func oraclePath(name string) string { return filepath.Join(oracleDir, name) }

func regenEnabled() bool { return os.Getenv("SPLASH_ORACLE_REGEN") == "1" }

// floatInputs returns the 1024 deterministic float64 inputs used by the
// floor/ceil/round oracles. Layout:
//
//	idx [0..200]    integers -100..100
//	idx [201..401]  halves  -100.5..100.5
//	idx [402..801]  uniformly spaced doubles in [-50, 50]
//	idx [802..821]  ±epsilon variants near 0 and ±1
//	idx [822..1023] negative half-integers and other parity fixtures
func floatInputs() []float64 {
	xs := make([]float64, 0, 1024)
	for i := -100; i <= 100; i++ {
		xs = append(xs, float64(i))
	}
	for i := -100; i <= 100; i++ {
		xs = append(xs, float64(i)+0.5)
	}
	const N = 400
	for i := 0; i < N; i++ {
		t := float64(i)/float64(N-1)*100.0 - 50.0
		xs = append(xs, t)
	}
	xs = append(xs,
		0.0, math.SmallestNonzeroFloat64, -math.SmallestNonzeroFloat64,
		1e-15, -1e-15, 1e-9, -1e-9,
		1.0-1e-15, 1.0+1e-15, -1.0+1e-15, -1.0-1e-15,
		2.5, -2.5, 3.5, -3.5, 0.49999999999, -0.49999999999,
		0.50000000001, -0.50000000001, 1e-300,
	)
	for len(xs) < 1024 {
		i := len(xs)
		xs = append(xs, -float64(i)*0.25)
	}
	return xs[:1024]
}

func writeCSVHeader(w *bufio.Writer, hdr string) { fmt.Fprintln(w, hdr) }

func ensureDir(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(oracleDir, 0o755); err != nil {
		t.Fatalf("mkdir oracle: %v", err)
	}
}

// regenIntOracle writes "<x>,<int>" rows for an int->int helper.
func regenIntOracle(t *testing.T, name string, xs []int, fn func(int) int) {
	t.Helper()
	ensureDir(t)
	f, err := os.Create(oraclePath(name))
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()
	writeCSVHeader(w, "x,result")
	for _, x := range xs {
		fmt.Fprintf(w, "%d,%d\n", x, fn(x))
	}
}

// regenFloatToIntOracle writes "<bits-hex>,<int>" rows for a float64->int helper.
// Float bits are stored as 16-hex so round-trip equality is exact (no decimal drift).
func regenFloatToIntOracle(t *testing.T, name string, xs []float64, fn func(float64) int) {
	t.Helper()
	ensureDir(t)
	f, err := os.Create(oraclePath(name))
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()
	writeCSVHeader(w, "bits,result")
	for _, x := range xs {
		fmt.Fprintf(w, "%016x,%d\n", math.Float64bits(x), fn(x))
	}
}

// regenAAGammaOracle writes "<i>,<lut[i]>" for the 17-entry coverage LUT.
func regenAAGammaOracle(t *testing.T) {
	t.Helper()
	ensureDir(t)
	lut := AAGamma()
	f, err := os.Create(oraclePath("aa_gamma.csv"))
	if err != nil {
		t.Fatalf("create aa_gamma: %v", err)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()
	writeCSVHeader(w, "i,result")
	for i, v := range lut {
		fmt.Fprintf(w, "%d,%d\n", i, v)
	}
}

// readIntOracle returns the rows of an int,int CSV (skipping header).
func readIntOracle(t *testing.T, name string) [][2]int {
	t.Helper()
	f, err := os.Open(oraclePath(name))
	if err != nil {
		t.Fatalf("open %s: %v (set SPLASH_ORACLE_REGEN=1 to regenerate)", name, err)
	}
	defer f.Close()
	var rows [][2]int
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	first := true
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if first {
			first = false
			continue
		}
		parts := strings.SplitN(line, ",", 2)
		if len(parts) != 2 {
			t.Fatalf("%s: bad row %q", name, line)
		}
		x, err := strconv.Atoi(parts[0])
		if err != nil {
			t.Fatalf("%s: bad x %q: %v", name, parts[0], err)
		}
		r, err := strconv.Atoi(parts[1])
		if err != nil {
			t.Fatalf("%s: bad result %q: %v", name, parts[1], err)
		}
		rows = append(rows, [2]int{x, r})
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("%s: scan: %v", name, err)
	}
	return rows
}

// readFloatToIntOracle returns the rows of a hexbits,int CSV.
func readFloatToIntOracle(t *testing.T, name string) []struct {
	X float64
	R int
} {
	t.Helper()
	f, err := os.Open(oraclePath(name))
	if err != nil {
		t.Fatalf("open %s: %v (set SPLASH_ORACLE_REGEN=1 to regenerate)", name, err)
	}
	defer f.Close()
	var rows []struct {
		X float64
		R int
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	first := true
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if first {
			first = false
			continue
		}
		parts := strings.SplitN(line, ",", 2)
		if len(parts) != 2 {
			t.Fatalf("%s: bad row %q", name, line)
		}
		bits, err := strconv.ParseUint(parts[0], 16, 64)
		if err != nil {
			t.Fatalf("%s: bad bits %q: %v", name, parts[0], err)
		}
		r, err := strconv.Atoi(parts[1])
		if err != nil {
			t.Fatalf("%s: bad result %q: %v", name, parts[1], err)
		}
		rows = append(rows, struct {
			X float64
			R int
		}{math.Float64frombits(bits), r})
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("%s: scan: %v", name, err)
	}
	return rows
}

// -------------------- Div255 --------------------

func TestDiv255MatchesFormula(t *testing.T) {
	// Spot checks at boundary values.
	cases := []struct{ x, want int }{
		{0, 0},
		{255, 1},
		{127, 0},
		{128, 1},
		{254, 1},
		{32385, 127}, // 255*127
		{32640, 128}, // 255*128
		{65025, 255}, // 255*255 — top of the documented input range
	}
	for _, c := range cases {
		if got := Div255(c.x); got != c.want {
			t.Errorf("Div255(%d) = %d, want %d", c.x, got, c.want)
		}
	}
}

func TestAllDiv255InputsMatchOracle(t *testing.T) {
	const n = 65536
	path := oraclePath("div255.csv")
	if _, err := os.Stat(path); err != nil {
		if !regenEnabled() {
			t.Fatalf("oracle %s missing; set SPLASH_ORACLE_REGEN=1 to generate", path)
		}
		xs := make([]int, n)
		for i := 0; i < n; i++ {
			xs[i] = i
		}
		regenIntOracle(t, "div255.csv", xs, Div255)
	}
	rows := readIntOracle(t, "div255.csv")
	if len(rows) != n {
		t.Fatalf("div255 oracle: got %d rows, want %d", len(rows), n)
	}
	for _, r := range rows {
		if got := Div255(r[0]); got != r[1] {
			t.Errorf("Div255(%d) = %d, oracle says %d", r[0], got, r[1])
		}
	}
}

// -------------------- Floor / Ceil / Round --------------------

func runFloatOracleTest(t *testing.T, name string, fn func(float64) int) {
	t.Helper()
	xs := floatInputs()
	if _, err := os.Stat(oraclePath(name)); err != nil {
		if !regenEnabled() {
			t.Fatalf("oracle %s missing; set SPLASH_ORACLE_REGEN=1 to generate", name)
		}
		regenFloatToIntOracle(t, name, xs, fn)
	}
	rows := readFloatToIntOracle(t, name)
	if len(rows) != len(xs) {
		t.Fatalf("%s: got %d rows, want %d", name, len(rows), len(xs))
	}
	for i, r := range rows {
		if got := fn(r.X); got != r.R {
			t.Errorf("%s row %d: fn(%v) = %d, oracle says %d", name, i, r.X, got, r.R)
		}
	}
}

func TestSplashFloorMatchesOracle(t *testing.T)  { runFloatOracleTest(t, "floor.csv", Floor) }
func TestSplashCeilMatchesOracle(t *testing.T)   { runFloatOracleTest(t, "ceil.csv", Ceil) }
func TestSplashRoundMatchesOracle(t *testing.T)  { runFloatOracleTest(t, "round.csv", Round) }

// -------------------- Properties (03_aa_scanner.md §9 #1, #2, #6) --------------------

// §9.1 — Floor truncates toward -infinity, NOT toward zero.
func TestSplashFloorTruncatesTowardNegInf(t *testing.T) {
	cases := []struct {
		x    float64
		want int
	}{
		{-0.5, -1}, {-0.0001, -1}, {-1.0, -1}, {-1.9999, -2},
		{0.0, 0}, {0.5, 0}, {0.9999, 0}, {1.0, 1},
		{-100.5, -101}, {100.5, 100},
	}
	for _, c := range cases {
		if got := Floor(c.x); got != c.want {
			t.Errorf("Floor(%v) = %d, want %d (must truncate toward -inf)", c.x, got, c.want)
		}
	}
}

// §9.2 — Round is floor(x+0.5); halve-away-from-zero is wrong.
func TestSplashRoundIsFloorPlusHalf(t *testing.T) {
	cases := []struct {
		x    float64
		want int
	}{
		{0.5, 1},
		{-0.5, 0}, // floor(-0.5+0.5) = floor(0) = 0
		{1.5, 2},
		{-1.5, -1}, // floor(-1.5+0.5) = floor(-1) = -1
		{2.5, 3},
		{-2.5, -2},
		{0.0, 0},
		{-0.0001, 0},
		{0.4999999, 0},
	}
	for _, c := range cases {
		if got := Round(c.x); got != c.want {
			t.Errorf("Round(%v) = %d, want %d (Round must be Floor(x+0.5))", c.x, got, c.want)
		}
	}
	// Cross-check against the spec relationship for every fuzzed input.
	for _, x := range floatInputs() {
		if math.IsInf(x+0.5, 0) || math.IsNaN(x+0.5) {
			continue
		}
		if got, want := Round(x), Floor(x+0.5); got != want {
			t.Fatalf("Round(%v) = %d != Floor(x+0.5) = %d", x, got, want)
		}
	}
}

// §9.6 — aaGamma[0]==0, aaGamma[16]==255, aaGamma[8]==90.
func TestAAGammaEndpointsAndMid(t *testing.T) {
	lut := AAGamma()
	if lut[0] != 0 {
		t.Errorf("aaGamma[0] = %d, want 0", lut[0])
	}
	if lut[16] != 255 {
		t.Errorf("aaGamma[16] = %d, want 255", lut[16])
	}
	if lut[8] != 90 {
		t.Errorf("aaGamma[8] = %d, want 90 (round(pow(0.5, 1.5)*255))", lut[8])
	}
}

func TestAAGammaMatchesOracle(t *testing.T) {
	path := oraclePath("aa_gamma.csv")
	if _, err := os.Stat(path); err != nil {
		if !regenEnabled() {
			t.Fatalf("oracle %s missing; set SPLASH_ORACLE_REGEN=1 to generate", path)
		}
		regenAAGammaOracle(t)
	}
	rows := readIntOracle(t, "aa_gamma.csv")
	if len(rows) != aaGammaLen {
		t.Fatalf("aa_gamma oracle: got %d rows, want %d", len(rows), aaGammaLen)
	}
	lut := AAGamma()
	for _, r := range rows {
		if r[0] < 0 || r[0] >= aaGammaLen {
			t.Fatalf("aa_gamma: out-of-range index %d", r[0])
		}
		if got := int(lut[r[0]]); got != r[1] {
			t.Errorf("aaGamma[%d] = %d, oracle says %d", r[0], got, r[1])
		}
	}
}

// AvgIsArithMean is a documentation-style invariant: splashAvg is
// 0.5*(x+y), NOT (x+y)/2 (the order matters for some FP edge cases at
// the exponent boundary, though for typical Splash inputs it does not).
func TestSplashAvgIsArithMean(t *testing.T) {
	cases := [][3]float64{
		{0, 0, 0},
		{1, 3, 2},
		{-2, 2, 0},
		{0.5, 1.5, 1.0},
		{-1.5, 0.5, -0.5},
	}
	for _, c := range cases {
		if got := Avg(c[0], c[1]); got != c[2] {
			t.Errorf("Avg(%v,%v) = %v, want %v", c[0], c[1], got, c[2])
		}
	}
}

// TestSplashCeilSpotChecks pins behaviors used by the scanner.
func TestSplashCeilSpotChecks(t *testing.T) {
	cases := []struct {
		x    float64
		want int
	}{
		{0.0, 0}, {0.0001, 1}, {1.0, 1}, {1.0001, 2},
		{-0.0001, 0}, {-1.0, -1}, {-1.9999, -1},
	}
	for _, c := range cases {
		if got := Ceil(c.x); got != c.want {
			t.Errorf("Ceil(%v) = %d, want %d", c.x, got, c.want)
		}
	}
}
