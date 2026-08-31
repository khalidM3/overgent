package multilang

import (
	"context"
	"os"
	"testing"

	"github.com/stickguy/stickguy/internal/contract"
)

// benchFiles pairs each fixture with the extractor that owns it, so the wasm
// numbers and the production numbers are measured over comparable real files.
var benchFiles = []struct {
	fixture string
	path    string
	wasm    bool
}{
	{"dataclasses.py", "lib/dataclasses.py", true},
	{"argparse.py", "lib/argparse.py", true},
	{"uri.js", "src/uri.js", true},
	{"convertPathData.js", "plugins/convertPathData.js", true},
	{"typescript-sample.ts.txt", "packages/coordination/src/intelligence.ts", true},
	{"typescript.go.txt", "internal/contract/typescript.go", false},
	{"typescript-sample.ts.txt", "packages/coordination/src/intelligence.ts", false},
}

// BenchmarkExtract measures per-file extraction latency. The wasm cases run the
// tree-sitter extractor; the others run the production internal/contract
// extractor on this repository's own Go and TypeScript sources.
func BenchmarkExtract(b *testing.B) {
	extractor := newExtractor(b)
	ctx := context.Background()
	for _, file := range benchFiles {
		source, err := os.ReadFile("../testdata/" + file.fixture)
		if err != nil {
			b.Fatal(err)
		}
		label := file.fixture
		if file.wasm {
			label = "wasm/" + label
		} else {
			label = "native/" + label
		}
		b.Run(label, func(b *testing.B) {
			b.SetBytes(int64(len(source)))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if file.wasm {
					if _, ok := extractor.Extract(ctx, file.path, source, nil); !ok {
						b.Fatal("no fingerprint")
					}
					continue
				}
				if _, ok := contract.Extract(file.path, source, nil); !ok {
					b.Fatal("no fingerprint")
				}
			}
		})
	}
}

// BenchmarkRuntimeStartup measures the one-time cost of compiling the wasm
// module and resolving both grammars' symbol tables. In the service this is
// paid once at process start, not per file.
func BenchmarkRuntimeStartup(b *testing.B) {
	ctx := context.Background()
	b.Run("compiler", func(b *testing.B) {
		for range b.N {
			extractor, err := New(ctx, mustModule(b), false)
			if err != nil {
				b.Fatal(err)
			}
			_ = extractor.Close(ctx)
		}
	})
	b.Run("interpreter", func(b *testing.B) {
		for range b.N {
			extractor, err := New(ctx, mustModule(b), true)
			if err != nil {
				b.Fatal(err)
			}
			_ = extractor.Close(ctx)
		}
	})
}

// BenchmarkInterpreterExtract shows the cost of the fallback wazero mode, which
// is what runs on any GOARCH the wazero compiler does not support.
func BenchmarkInterpreterExtract(b *testing.B) {
	extractor, err := New(context.Background(), mustModule(b), true)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = extractor.Close(context.Background()) })
	source, err := os.ReadFile("../testdata/dataclasses.py")
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(source)))
	b.ReportAllocs()
	for range b.N {
		if _, ok := extractor.Extract(context.Background(), "lib/dataclasses.py", source, nil); !ok {
			b.Fatal("no fingerprint")
		}
	}
}
