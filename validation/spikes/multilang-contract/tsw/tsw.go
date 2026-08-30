// Package tsw runs a tree-sitter build compiled to wasm32-wasi under wazero.
// It exists to answer one question for the contract spike: can a pure-Go
// process (no CGO, no Node) get a real parse tree for arbitrary languages?
//
// The binding is deliberately thin. The guest exposes one bulk entry point,
// sg_dump, which performs the whole bounded pre-order walk inside wasm and
// writes a packed record array. The host therefore crosses the wasm boundary a
// fixed number of times per file instead of once per node accessor, which is
// what makes per-file latency comparable to a native extractor.
package tsw

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// recordWords is the number of uint32 fields in the guest's SGRecord. The
// guest reports its own sizeof so a layout drift fails loudly at startup
// rather than silently misreading the dump.
const recordWords = 6

// Record is one node of the bounded pre-order walk. Start and End index the
// source the caller already holds; no source text crosses back from the guest.
type Record struct {
	Symbol uint32
	Start  uint32
	End    uint32
	Depth  uint32
	Field  uint32
	Named  bool
}

// Runtime owns the wazero instance and the guest heap allocations that are
// reused across files. It is not safe for concurrent use; callers hold one per
// worker or serialize through Extract.
type Runtime struct {
	mu      sync.Mutex
	runtime wazero.Runtime
	module  api.Module

	malloc, free                   api.Function
	parserNew, parserDelete        api.Function
	parserSetLanguage, parserParse api.Function
	treeDelete                     api.Function
	symbolCount, symbolName        api.Function
	dump, hasError                 api.Function

	parser uint64
	langs  map[string]*Language
	srcPtr uint32
	srcCap uint32
	recPtr uint32
	recCap uint32
}

// Language is a grammar linked into the wasm module, with its symbol table
// resolved once at load time so the hot path never reads guest strings.
type Language struct {
	name    string
	pointer uint64
	symbols []string
}

// Name reports the tree-sitter node type for a symbol id, or "" when the id is
// outside the grammar's table.
func (l *Language) Name(symbol uint32) string {
	if int(symbol) >= len(l.symbols) {
		return ""
	}
	return l.symbols[symbol]
}

// Config bounds one Runtime. SourceBytes and Records size the two guest
// buffers that are allocated once and reused.
type Config struct {
	SourceBytes uint32
	Records     uint32
	Interpreter bool
}

// New instantiates the module and resolves each named grammar. names are
// tree-sitter language names ("python"), matching the tree_sitter_<name>
// exports the wasm was linked with.
func New(ctx context.Context, wasmModule []byte, names []string, config Config) (*Runtime, error) {
	if config.SourceBytes == 0 {
		config.SourceBytes = 1 << 20
	}
	if config.Records == 0 {
		config.Records = 1 << 17
	}
	runtimeConfig := wazero.NewRuntimeConfig()
	if config.Interpreter {
		runtimeConfig = wazero.NewRuntimeConfigInterpreter()
	}
	wasmRuntime := wazero.NewRuntimeWithConfig(ctx, runtimeConfig)
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, wasmRuntime); err != nil {
		return nil, fmt.Errorf("instantiating wasi: %w", err)
	}
	module, err := wasmRuntime.Instantiate(ctx, wasmModule)
	if err != nil {
		return nil, fmt.Errorf("instantiating module: %w", err)
	}
	r := &Runtime{runtime: wasmRuntime, module: module, langs: map[string]*Language{}}
	lookup := func(name string) (api.Function, error) {
		function := module.ExportedFunction(name)
		if function == nil {
			return nil, fmt.Errorf("wasm module does not export %q", name)
		}
		return function, nil
	}
	for target, name := range map[*api.Function]string{
		&r.malloc: "malloc", &r.free: "free",
		&r.parserNew: "ts_parser_new", &r.parserDelete: "ts_parser_delete",
		&r.parserSetLanguage: "ts_parser_set_language", &r.parserParse: "ts_parser_parse_string",
		&r.treeDelete:  "ts_tree_delete",
		&r.symbolCount: "ts_language_symbol_count", &r.symbolName: "ts_language_symbol_name",
		&r.dump: "sg_dump", &r.hasError: "sg_tree_has_error",
	} {
		function, err := lookup(name)
		if err != nil {
			return nil, err
		}
		*target = function
	}
	size, err := call(ctx, module.ExportedFunction("sg_record_size"))
	if err != nil {
		return nil, err
	}
	if size != recordWords*4 {
		return nil, fmt.Errorf("guest record size %d does not match host layout %d", size, recordWords*4)
	}
	if r.parser, err = call(ctx, r.parserNew); err != nil {
		return nil, fmt.Errorf("creating parser: %w", err)
	}
	if r.srcPtr, err = r.alloc(ctx, config.SourceBytes); err != nil {
		return nil, err
	}
	r.srcCap = config.SourceBytes
	if r.recPtr, err = r.alloc(ctx, config.Records*recordWords*4); err != nil {
		return nil, err
	}
	r.recCap = config.Records
	for _, name := range names {
		language, err := r.loadLanguage(ctx, name)
		if err != nil {
			return nil, err
		}
		r.langs[name] = language
	}
	return r, nil
}

// Close releases the wazero runtime and everything allocated inside it.
func (r *Runtime) Close(ctx context.Context) error {
	return r.runtime.Close(ctx)
}

// Language returns a loaded grammar by tree-sitter name.
func (r *Runtime) Language(name string) (*Language, bool) {
	language, ok := r.langs[name]
	return language, ok
}

func (r *Runtime) loadLanguage(ctx context.Context, name string) (*Language, error) {
	constructor := r.module.ExportedFunction("tree_sitter_" + name)
	if constructor == nil {
		return nil, fmt.Errorf("wasm module has no grammar %q", name)
	}
	pointer, err := call(ctx, constructor)
	if err != nil {
		return nil, fmt.Errorf("initializing grammar %q: %w", name, err)
	}
	count, err := call(ctx, r.symbolCount, pointer)
	if err != nil {
		return nil, fmt.Errorf("reading symbol count for %q: %w", name, err)
	}
	symbols := make([]string, count)
	for id := range symbols {
		stringPointer, err := call(ctx, r.symbolName, pointer, uint64(id))
		if err != nil {
			return nil, fmt.Errorf("reading symbol name for %q: %w", name, err)
		}
		symbols[id] = r.readCString(uint32(stringPointer))
	}
	return &Language{name: name, pointer: pointer, symbols: symbols}, nil
}

// ErrTooLarge reports a source or dump that exceeds the configured bounds.
var ErrTooLarge = errors.New("input exceeds configured bounds")

// Parse parses source with language and appends the bounded pre-order walk to
// dst. It reports whether the tree is free of ERROR and MISSING nodes. Callers
// that require a clean parse treat false as "no fingerprint".
func (r *Runtime) Parse(ctx context.Context, language *Language, source []byte, maxDepth uint32, dst []Record) ([]Record, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if uint32(len(source)) > r.srcCap {
		return dst, false, ErrTooLarge
	}
	if !r.module.Memory().Write(r.srcPtr, source) {
		return dst, false, errors.New("writing source into guest memory")
	}
	if _, err := call(ctx, r.parserSetLanguage, r.parser, language.pointer); err != nil {
		return dst, false, fmt.Errorf("setting language: %w", err)
	}
	tree, err := call(ctx, r.parserParse, r.parser, 0, uint64(r.srcPtr), uint64(len(source)))
	if err != nil {
		return dst, false, fmt.Errorf("parsing: %w", err)
	}
	if tree == 0 {
		return dst, false, errors.New("parser returned no tree")
	}
	defer func() { _, _ = call(ctx, r.treeDelete, tree) }()
	bad, err := call(ctx, r.hasError, tree)
	if err != nil {
		return dst, false, fmt.Errorf("checking parse errors: %w", err)
	}
	count, err := call(ctx, r.dump, tree, uint64(maxDepth), uint64(r.recPtr), uint64(r.recCap))
	if err != nil {
		return dst, false, fmt.Errorf("dumping tree: %w", err)
	}
	if uint32(count) > r.recCap {
		return dst, false, ErrTooLarge
	}
	raw, ok := r.module.Memory().Read(r.recPtr, uint32(count)*recordWords*4)
	if !ok {
		return dst, false, errors.New("reading dump from guest memory")
	}
	for offset := 0; offset+recordWords*4 <= len(raw); offset += recordWords * 4 {
		word := func(index int) uint32 {
			base := offset + index*4
			return uint32(raw[base]) | uint32(raw[base+1])<<8 | uint32(raw[base+2])<<16 | uint32(raw[base+3])<<24
		}
		dst = append(dst, Record{
			Symbol: word(0), Start: word(1), End: word(2),
			Depth: word(3), Field: word(4), Named: word(5) == 1,
		})
	}
	return dst, bad == 0, nil
}

func (r *Runtime) alloc(ctx context.Context, size uint32) (uint32, error) {
	pointer, err := call(ctx, r.malloc, uint64(size))
	if err != nil {
		return 0, fmt.Errorf("allocating %d guest bytes: %w", size, err)
	}
	if pointer == 0 {
		return 0, fmt.Errorf("guest refused an allocation of %d bytes", size)
	}
	return uint32(pointer), nil
}

func (r *Runtime) readCString(pointer uint32) string {
	memory := r.module.Memory()
	var out []byte
	for offset := pointer; ; offset++ {
		b, ok := memory.ReadByte(offset)
		if !ok || b == 0 {
			return string(out)
		}
		out = append(out, b)
	}
}

func call(ctx context.Context, function api.Function, args ...uint64) (uint64, error) {
	if function == nil {
		return 0, errors.New("missing export")
	}
	results, err := function.Call(ctx, args...)
	if err != nil {
		return 0, err
	}
	if len(results) == 0 {
		return 0, nil
	}
	return results[0], nil
}
