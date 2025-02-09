package gosseract

import (
	"bytes"
	"context"
	"embed"
	"io/fs"
	"log"
	"math"
	"os"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/experimental/logging"
	"github.com/tetratelabs/wazero/imports/emscripten"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

//go:embed build/tesseract-core.wasm
var binary []byte

var compiledModule wazero.CompiledModule
var runtimeConfig wazero.RuntimeConfig
var r wazero.Runtime
var ctx context.Context
var initLock = &sync.Mutex{}

//go:embed eng.traineddata
var languages embed.FS

func newApi() *tesseractApi {
	return newApiWithFS(nil)
}

func newApiWithFS(fs fs.FS) *tesseractApi {
	func() {
		initLock.Lock()
		defer initLock.Unlock()
		if r == nil {
			ctx = context.WithValue(context.Background(), experimental.FunctionListenerFactoryKey{}, logging.NewLoggingListenerFactory(os.Stdout))
			ctx = context.Background() // Comment this line to get debug information.

			if runtimeConfig == nil {
				cache := wazero.NewCompilationCache()
				runtimeConfig = wazero.NewRuntimeConfig().WithCompilationCache(cache)
			}

			// Create a new WebAssembly Runtime.
			r = wazero.NewRuntimeWithConfig(ctx, runtimeConfig)

			wasi_snapshot_preview1.MustInstantiate(ctx, r)

			if compiledModule == nil {
				module, err := r.CompileModule(ctx, binary)
				compiledModule = module
				if err != nil {
					log.Panicf("failed to compile module: %v", err)
				}
			}
			_, err := emscripten.InstantiateForModule(ctx, r, compiledModule)
			if err != nil {
				log.Panicf("failed to instantiate module (emscripten): %v", err)
			}

		}
	}()

	mod, err := r.InstantiateModule(ctx, compiledModule, wazero.NewModuleConfig().
		WithStartFunctions("_initialize").
		WithFSConfig(
			wazero.NewFSConfig().
				WithDirMount("/", "/").
				WithFSMount(languages, "/tessdata/").
				WithFSMount(fs, "/custom/")))

	if err != nil {
		log.Panicf("failed to instantiate module: %v", err)
	}
	tAPI := tesseractApi{
		module:                   mod,
		context:                  ctx,
		Create:                   fun(ctx, mod, "Create"),
		Free:                     fun(ctx, mod, "Free"),
		free:                     fun(ctx, mod, "free"),
		malloc:                   fun(ctx, mod, "malloc"),
		Clear:                    fun(ctx, mod, "Clear"),
		ClearPersistentCache:     fun(ctx, mod, "ClearPersistentCache"),
		Init:                     fun(ctx, mod, "Init"),
		GetBoundingBoxes:         fun(ctx, mod, "GetBoundingBoxes"),
		GetBoundingBoxesVerbose:  fun(ctx, mod, "GetBoundingBoxesVerbose"),
		SetVariable:              fun(ctx, mod, "SetVariable"),
		SetPixImage:              fun(ctx, mod, "SetPixImage"),
		SetPageSegMode:           fun(ctx, mod, "SetPageSegMode"),
		GetPageSegMode:           fun(ctx, mod, "GetPageSegMode"),
		Utf8Text:                 fun(ctx, mod, "UTF8Text"),
		HocrText:                 fun(ctx, mod, "HOCRText"),
		Version:                  fun(ctx, mod, "Version"),
		GetDataPath:              fun(ctx, mod, "GetDataPath"),
		CreatePixImageByFilepath: fun(ctx, mod, "CreatePixImageByFilePath"),
		CreatePixImageFromBytes:  fun(ctx, mod, "CreatePixImageFromBytes"),
		DestroyPixImage:          fun(ctx, mod, "DestroyPixImage"),
		FileExists:               fun(ctx, mod, "FileExists"),
	}

	// try calling file exists method, to check if everything is working
	_, err = mod.ExportedFunction("FileExists").Call(ctx, 0)
	if err != nil {
		log.Panicln("could not load wasm module")
	}

	return &tAPI
}

func fun(ctx context.Context, mod api.Module, name string) func(params ...uint64) []uint64 {
	funDef := mod.ExportedFunction(name)

	return func(params ...uint64) []uint64 {
		r, err := funDef.Call(ctx, params...)
		if err != nil {
			panic(err)
		}
		return r
	}
}

type tesseractApi struct {
	module  api.Module
	context context.Context
	Create,
	Free,
	free,
	malloc,
	Clear,
	ClearPersistentCache,
	Init,
	GetBoundingBoxes,
	GetBoundingBoxesVerbose,
	SetVariable,
	SetPixImage,
	SetPageSegMode,
	GetPageSegMode,
	Utf8Text,
	HocrText,
	Version,
	FileExists,
	GetDataPath,
	CreatePixImageByFilepath,
	CreatePixImageFromBytes,
	DestroyPixImage func(params ...uint64) []uint64
}

func (t *tesseractApi) Close() {
	t.module.Close(t.context)
}

func (t *tesseractApi) ReadString(ptr uint64) string {
	if ptr == 0 || ptr == 0xffffffff {
		return ""
	}
	mem := t.module.Memory()
	buf, ok := mem.Read(uint32(ptr), math.MaxUint32)
	if !ok {
		buf, ok = mem.Read(uint32(ptr), mem.Size()-uint32(ptr))
		if !ok {
			panic("range error")
		}
	}
	if i := bytes.IndexByte(buf, 0); i < 0 {
		panic("string is not null terminated")
	} else {
		return string(buf[:i])
	}
}
