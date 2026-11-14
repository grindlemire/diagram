package diagramtest

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/grindlemire/diagram"
)

// fakeTB records failures instead of failing the real test.
type fakeTB struct {
	testing.TB
	name   string
	errors []string
	logs   []string
	fatal  bool
}

func (f *fakeTB) Helper()      {}
func (f *fakeTB) Name() string { return f.name }
func (f *fakeTB) Errorf(format string, args ...any) {
	f.errors = append(f.errors, fmt.Sprintf(format, args...))
}
func (f *fakeTB) Logf(format string, args ...any) {
	f.logs = append(f.logs, fmt.Sprintf(format, args...))
}
func (f *fakeTB) Fatalf(format string, args ...any) {
	f.Errorf(format, args...)
	f.fatal = true
	runtime.Goexit()
}

// golden runs Golden with a fake TB named name, in a goroutine so Fatalf
// can stop it.
func golden(t *testing.T, name string, d *diagram.Diagram, opts ...Option) *fakeTB {
	f := &fakeTB{TB: t, name: name}
	done := make(chan struct{})
	go func() {
		defer close(done)
		Golden(f, d, opts...)
	}()
	<-done
	return f
}

func sample(label string) *diagram.Diagram {
	d := diagram.New("Sample")
	a := d.Node(label)
	b := d.Node("Store", diagram.Cylinder)
	d.Connect(a, b, diagram.Label("writes"))
	return d
}

func setUpdate(t *testing.T, on bool) {
	old := *updateFlag
	*updateFlag = on
	t.Cleanup(func() { *updateFlag = old })
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestGoldenMatch(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("DIAGRAM_UPDATE", "1")
	f := golden(t, "TestPayments", sample("API"), Formats("svg", "drawio", "png", "html"))
	if len(f.errors) > 0 {
		t.Fatalf("update failed: %v", f.errors)
	}
	for _, ext := range []string{"svg", "drawio", "png", "html"} {
		if !exists("testdata/TestPayments." + ext) {
			t.Errorf("testdata/TestPayments.%s not written", ext)
		}
	}
	t.Setenv("DIAGRAM_UPDATE", "")

	// The real test passes against the files just written.
	Golden(t, sample("API"), Name("TestPayments"), Formats("svg", "drawio", "png", "html"))
	if f := golden(t, "TestPayments", sample("API"), Formats("svg", "drawio", "png", "html")); len(f.errors) > 0 {
		t.Errorf("match failed: %v", f.errors)
	}
}

func TestGoldenMismatch(t *testing.T) {
	t.Chdir(t.TempDir())
	setUpdate(t, true)
	golden(t, "TestFlow/edge_case", sample("API"))
	setUpdate(t, false)

	f := golden(t, "TestFlow/edge_case", sample("Gateway"))
	if len(f.errors) != 1 || f.fatal {
		t.Fatalf("want one non-fatal error, got %d (fatal %v): %v", len(f.errors), f.fatal, f.errors)
	}
	msg := f.errors[0]
	for _, want := range []string{
		"testdata/TestFlow-edge_case.svg differs from the rendered diagram",
		"first difference at line",
		"wrote the new output to testdata/TestFlow-edge_case.got.svg",
		"open both files side by side",
		"-diagram.update", "DIAGRAM_UPDATE=1",
		"go test -run '^TestFlow$/^edge_case$' -diagram.update",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q:\n%s", want, msg)
		}
	}
	got, err := os.ReadFile("testdata/TestFlow-edge_case.got.svg")
	if err != nil {
		t.Fatal(err)
	}
	var want bytes.Buffer
	sample("Gateway").RenderSVG(&want)
	if !bytes.Equal(got, want.Bytes()) {
		t.Error(".got.svg is not the new output")
	}

	// Passing again removes the stale .got file.
	if f := golden(t, "TestFlow/edge_case", sample("API")); len(f.errors) > 0 {
		t.Fatal(f.errors)
	}
	if exists("testdata/TestFlow-edge_case.got.svg") {
		t.Error("stale .got.svg not removed")
	}
}

func TestGoldenMissing(t *testing.T) {
	t.Chdir(t.TempDir())
	f := golden(t, "TestNew", sample("API"), Formats("drawio"))
	if len(f.errors) != 1 || !strings.Contains(f.errors[0], "testdata/TestNew.drawio does not exist") {
		t.Fatalf("errors: %v", f.errors)
	}
	if !exists("testdata/TestNew.got.drawio") {
		t.Error(".got.drawio not written")
	}
}

func TestGoldenUpdate(t *testing.T) {
	t.Chdir(t.TempDir())
	setUpdate(t, true)
	golden(t, "TestX", sample("API"), Name("nested/flow"))
	setUpdate(t, false)
	os.WriteFile("testdata/nested/flow.got.svg", []byte("stale"), 0o644)

	// The flag rewrites a changed golden file.
	setUpdate(t, true)
	f := golden(t, "TestX", sample("Gateway"), Name("nested/flow"))
	setUpdate(t, false)
	if len(f.errors) > 0 {
		t.Fatal(f.errors)
	}
	if len(f.logs) != 1 || !strings.Contains(f.logs[0], filepath.Join("testdata", "nested", "flow.svg")) {
		t.Errorf("logs: %v", f.logs)
	}
	if exists("testdata/nested/flow.got.svg") {
		t.Error("update left the .got file")
	}
	if f := golden(t, "TestX", sample("Gateway"), Name("nested/flow")); len(f.errors) > 0 {
		t.Errorf("not updated: %v", f.errors)
	}

	// So does DIAGRAM_UPDATE.
	t.Setenv("DIAGRAM_UPDATE", "true")
	golden(t, "TestX", sample("API"), Name("nested/flow"))
	t.Setenv("DIAGRAM_UPDATE", "0")
	if f := golden(t, "TestX", sample("API"), Name("nested/flow")); len(f.errors) > 0 {
		t.Errorf("DIAGRAM_UPDATE did not update: %v", f.errors)
	}
}

func TestGoldenInvalid(t *testing.T) {
	t.Chdir(t.TempDir())
	d := diagram.New("Broken")
	g := d.Grid("Workers", 2)
	g.Node("A", 0, 0)
	g.Node("B", 0, 0)
	f := golden(t, "TestBroken", d)
	if !f.fatal || len(f.errors) != 1 {
		t.Fatalf("want a fatal error, got %v", f.errors)
	}
	for _, want := range []string{`diagram "Broken" is invalid`, "diagramtest_test.go:", "already occupied"} {
		if !strings.Contains(f.errors[0], want) {
			t.Errorf("error lacks %q:\n%s", want, f.errors[0])
		}
	}
	if exists("testdata") {
		t.Error("files written for an invalid diagram")
	}
}

func TestGoldenBadOptions(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, opt := range []Option{Formats("pdf"), Formats(), Name("../x"), Name("")} {
		if f := golden(t, "TestOpt", sample("API"), opt); !f.fatal {
			t.Errorf("option accepted: %v", f.errors)
		}
	}
	if f := golden(t, "TestNil", nil); !f.fatal {
		t.Error("nil diagram accepted")
	}
}

func TestSanitize(t *testing.T) {
	for in, want := range map[string]string{
		"TestFoo":                 "TestFoo",
		"TestFoo/case_one":        "TestFoo-case_one",
		"TestFoo/a:b#01":          "TestFoo-a_b_01",
		"TestFoo/payments.v2":     "TestFoo-payments.v2",
		"TestÜber/../..":          "Test_ber-..-",
		"..":                      "diagram",
		"TestFoo/with spaces (1)": "TestFoo-with_spaces__1_",
	} {
		if got := sanitize(in); got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestComparePNG(t *testing.T) {
	enc := func(img image.Image) []byte {
		var b bytes.Buffer
		png.Encode(&b, img)
		return b.Bytes()
	}
	base := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for i := range base.Pix {
		base.Pix[i] = 200
		if i%4 == 3 {
			base.Pix[i] = 255 // opaque
		}
	}
	near := image.NewRGBA(base.Rect)
	copy(near.Pix, base.Pix)
	for i := 0; i < len(near.Pix); i += 4 {
		near.Pix[i] = 210 // within tolerance everywhere
	}
	near.Set(5, 5, color.RGBA{0, 0, 0, 255}) // and one pixel far off
	if d := compare("png", enc(near), enc(base)); d != "" {
		t.Errorf("small differences failed: %s", d)
	}
	far := image.NewRGBA(base.Rect)
	copy(far.Pix, base.Pix)
	for x := range 20 {
		far.Set(x, 0, color.RGBA{0, 0, 0, 255})
	}
	if d := compare("png", enc(far), enc(base)); !strings.Contains(d, "20 of 10000 pixels differ") {
		t.Errorf("large difference: %q", d)
	}
	small := image.NewRGBA(image.Rect(0, 0, 10, 10))
	if d := compare("png", enc(small), enc(base)); !strings.Contains(d, "size") {
		t.Errorf("size difference: %q", d)
	}
}
