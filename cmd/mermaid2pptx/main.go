package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/serendipitynz/mermaid2pptx/internal/convert"
)

// version is injected at release build time via
// -ldflags "-X main.version=<tag>"; when empty it is derived from the build
// info embedded by the go toolchain (see versionString).
var version = ""

// config holds the parsed CLI options.
type config struct {
	out     string
	font    string
	force   bool
	margin  float64
	mmdc    string
	version bool
}

func main() {
	fs := flag.NewFlagSet("mermaid2pptx", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] input.{svg|mmd} [input2 ...]\n\nConverts mermaid diagrams (SVG, or .mmd via mermaid-cli) into editable PowerPoint slides.\nOptions may appear before, after, or between input files.\n\nOptions:\n", os.Args[0])
		fs.PrintDefaults()
	}
	cfg, inputs := parseArgs(fs, os.Args[1:])
	if cfg.version {
		fmt.Printf("mermaid2pptx %s\n", versionString())
		return
	}
	if len(inputs) == 0 {
		fs.Usage()
		os.Exit(2)
	}
	if cfg.out != "" && len(inputs) > 1 {
		fmt.Fprintln(os.Stderr, "error: -o cannot be used with multiple inputs")
		os.Exit(2)
	}

	for _, in := range inputs {
		dst := cfg.out
		if dst == "" {
			dst = strings.TrimSuffix(in, filepath.Ext(in)) + ".pptx"
		}
		if err := convertFile(in, dst, cfg.font, cfg.margin, cfg.force, cfg.mmdc); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s: %v\n", in, err)
			os.Exit(1)
		}
		fmt.Printf("%s -> %s\n", in, dst)
	}
}

// registerFlags binds the options onto fs and returns the destination config.
func registerFlags(fs *flag.FlagSet) *config {
	cfg := &config{}
	fs.StringVar(&cfg.out, "o", "", "output .pptx path (only with a single input; default: <input>.pptx)")
	fs.StringVar(&cfg.font, "font", "Noto Sans JP", "font family used for all text")
	fs.BoolVar(&cfg.force, "f", false, "overwrite the output file if it exists")
	fs.Float64Var(&cfg.margin, "margin", 0.3, "slide margin in inches")
	fs.StringVar(&cfg.mmdc, "mmdc", "", "path to the mermaid-cli binary used for .mmd inputs (default: mmdc in PATH)")
	fs.BoolVar(&cfg.version, "version", false, "print version information and exit")
	return cfg
}

// versionString reports the release version injected at build time, or falls
// back to the module version and VCS revision embedded by the go toolchain
// (a bare `go build` yields "(devel)" plus the commit; `go install m@vX` the tag).
func versionString() string {
	if version != "" {
		return version
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	// A tag (go install m@vX) or a VCS-derived pseudo-version (recent go build)
	// already carries the revision and dirty state, so print it as-is.
	if v := bi.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	// Older toolchains report "(devel)": augment with the embedded VCS revision.
	var rev, dirty string
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "-dirty"
			}
		}
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	if rev == "" {
		return "(devel)"
	}
	return fmt.Sprintf("(devel) %s%s", rev, dirty)
}

// parseArgs parses fs but lets options appear before, after, or between input
// files (Go's flag stops at the first non-flag argument). It parses, sets
// aside the leading positional, then re-parses the remainder until only
// positionals remain. Each fs.Parse resets fs.args, so this is safe.
func parseArgs(fs *flag.FlagSet, args []string) (*config, []string) {
	cfg := registerFlags(fs)
	var inputs []string
	for {
		if err := fs.Parse(args); err != nil {
			return cfg, inputs // ContinueOnError sets returns here; ExitOnError exits
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return cfg, inputs
		}
		inputs = append(inputs, rest[0])
		args = rest[1:]
	}
}

func convertFile(in, dst, font string, margin float64, force bool, mmdc string) error {
	if !force {
		if _, err := os.Stat(dst); err == nil {
			return fmt.Errorf("output %s already exists (use -f to overwrite)", dst)
		}
	}
	svgPath := in
	switch strings.ToLower(filepath.Ext(in)) {
	case ".mmd", ".mermaid":
		rendered, err := renderMermaid(in, mmdc)
		if err != nil {
			return err
		}
		defer os.Remove(rendered)
		svgPath = rendered
	}
	f, err := os.Open(svgPath)
	if err != nil {
		return err
	}
	defer f.Close()
	d, err := convert.ParseMermaidSVG(f)
	if err != nil {
		return err
	}
	slideXML := convert.GenerateSlideXML(d, convert.Options{Font: font, MarginIn: margin})
	o, err := os.Create(dst)
	if err != nil {
		return err
	}
	if err := convert.WritePPTX(o, slideXML); err != nil {
		o.Close()
		os.Remove(dst)
		return err
	}
	return o.Close()
}

// renderMermaid renders a .mmd file to a temporary SVG using mermaid-cli.
// The caller removes the returned file.
func renderMermaid(in, mmdc string) (string, error) {
	if mmdc == "" {
		p, err := exec.LookPath("mmdc")
		if err != nil {
			return "", fmt.Errorf(".mmd input requires mermaid-cli; install it with `npm install -g @mermaid-js/mermaid-cli`, pass -mmdc <path>, or convert to SVG yourself")
		}
		mmdc = p
	}
	tmp, err := os.CreateTemp("", "mermaid2pptx-*.svg")
	if err != nil {
		return "", err
	}
	tmp.Close()
	cmd := exec.Command(mmdc, "-i", in, "-o", tmp.Name(), "-I", "my-svg", "-b", "white")
	if outBytes, err := cmd.CombinedOutput(); err != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("mmdc failed: %v\n%s", err, outBytes)
	}
	return tmp.Name(), nil
}
