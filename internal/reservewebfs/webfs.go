package reservewebfs

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Bundle struct {
	Name string `json:"name"`
	SourceDir string `json:"source_dir"`
	Entry string `json:"entry"`
}

type Config struct {
	RuntimeWebDir string `json:"runtime_web_dir"`
	Bundles []Bundle `json:"bundles"`
}

// EnsureFlatWebFS builds a flat runtime/web filesystem.
// It copies each bundle's entry to <name>.index.html.
// It also copies other static files under SourceDir into the flat dir as content-hashed files:
//   <origbase>.<hash>.<ext>
// and rewrites the entry html by replacing occurrences of "{{asset:filename.ext}}" with the hashed output name.
func EnsureFlatWebFS(cfg Config) error {
	if cfg.RuntimeWebDir == "" {
		cfg.RuntimeWebDir = filepath.Join("runtime","web")
	}
	if err := os.MkdirAll(cfg.RuntimeWebDir, 0700); err != nil { return err }

	// clear old files (flat directory)
	ents, _ := os.ReadDir(cfg.RuntimeWebDir)
	for _, e := range ents {
		if e.IsDir() { continue }
		_ = os.Remove(filepath.Join(cfg.RuntimeWebDir, e.Name()))
	}

	for _, b := range cfg.Bundles {
		assetMap, err := copyHashedAssets(cfg.RuntimeWebDir, b.SourceDir)
		if err != nil { return err }

		src := filepath.Join(b.SourceDir, b.Entry)
		raw, err := os.ReadFile(src)
		if err != nil { return err }
		html := string(raw)
		for orig, hashed := range assetMap {
			h := "{{asset:" + orig + "}}"
			html = strings.ReplaceAll(html, h, hashed)
		}

		dst := filepath.Join(cfg.RuntimeWebDir, b.Name + ".index.html")
		if err := os.WriteFile(dst, []byte(html), 0644); err != nil { return err }
	}

	_ = os.WriteFile(filepath.Join(cfg.RuntimeWebDir, "web.manifest"), []byte(manifestHash(cfg)), 0644)
	return nil
}

func copyHashedAssets(runtimeDir, sourceDir string) (map[string]string, error) {
	out := map[string]string{}
	ents, err := os.ReadDir(sourceDir)
	if err != nil { return out, err }
	for _, e := range ents {
		if e.IsDir() { continue }
		name := e.Name()
		if strings.HasSuffix(name, ".html") { continue }
		p := filepath.Join(sourceDir, name)
		b, err := os.ReadFile(p)
		if err != nil { return out, err }
		h := sha256.Sum256(b)
		hexH := hex.EncodeToString(h[:])[:12]
		ext := filepath.Ext(name)
		base := strings.TrimSuffix(name, ext)
		dstName := base + "." + hexH + ext
		if err := os.WriteFile(filepath.Join(runtimeDir, dstName), b, 0644); err != nil { return out, err }
		out[name] = dstName
	}
	return out, nil
}

func manifestHash(cfg Config) string {
	h := sha256.New()
	h.Write([]byte(cfg.RuntimeWebDir))
	for _, b := range cfg.Bundles {
		h.Write([]byte("|"+b.Name+"|"+b.SourceDir+"|"+b.Entry))
	}
	return hex.EncodeToString(h.Sum(nil))
}

var _ = io.EOF
