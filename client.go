package gosseract

import (
	"fmt"
	"image"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Version returns the version of Tesseract-OCR
func Version() string {
	wasm := newApi()
	defer wasm.Close()
	api := wasm.Create()
	defer wasm.Free(api...)
	versionPtr := wasm.Version(api...)
	return wasm.ReadString(versionPtr[0])
}

// ClearPersistentCache clears any library-level memory caches. There are a variety of expensive-to-load constant data structures (mostly language dictionaries) that are cached globally – surviving the Init() and End() of individual TessBaseAPI's. This function allows the clearing of these caches.
func (client *Client) ClearPersistentCache() {
	client.wasm.ClearPersistentCache(client.api)
}

// Client is argument builder for tesseract::TessBaseAPI.
type Client struct {
	// the tesseract wasm binding.
	wasm *tesseractApi

	api uint64
	// Holds a reference to the pix image to be able to destroy on client close
	// or when a new image is set
	pixImage uint64

	// Trim specifies characters to trim, which would be trimed from result string.
	// As results of OCR, text often contains unnecessary characters, such as newlines, on the head/foot of string.
	// If `Trim` is set, this client will remove specified characters from the result.
	Trim bool

	// TessdataPrefix can indicate directory path to `tessdata`.
	// It is set `/usr/local/share/tessdata/` or something like that, as default.
	// TODO: Implement and test
	TessdataPrefix string

	// Languages are languages to be detected. If not specified, it's gonna be "eng".
	Languages []string

	// Variables is just a pool to evaluate "tesseract::TessBaseAPI->SetVariable" in delay.
	// TODO: Think if it should be public, or private property.
	Variables map[SettableVariable]string

	// Config is a file path to the configuration for Tesseract
	// See http://www.sk-spell.sk.cx/tesseract-ocr-parameters-in-302-version
	// TODO: Fix link to official page
	ConfigFilePath string

	// internal flag to check if the instance should be initialized again
	// i.e, we should create a new gosseract client when language or config file change
	shouldInit bool
}

// NewClient construct new Client. It's due to caller to Close this client.
func NewClient() *Client {
	wasm := newApi()
	client := &Client{
		wasm:       wasm,
		api:        wasm.Create()[0],
		Variables:  map[SettableVariable]string{},
		Trim:       true,
		shouldInit: true,
		Languages:  []string{"eng"},
	}
	return client
}

// NewClient construct new Client with a FS that will be mounted at '/custom/'.
// The file system can be used to provide (embedded) traineddata or other files to tesseract.
// It's due to caller to Close this client.
func NewClientWithFS(fs fs.FS) *Client {
	wasm := newApiWithFS(fs)
	client := &Client{
		wasm:       wasm,
		api:        wasm.Create()[0],
		Variables:  map[SettableVariable]string{},
		Trim:       true,
		shouldInit: true,
		Languages:  []string{"eng"},
	}
	return client
}

// Close frees allocated API. This MUST be called for ANY client constructed by "NewClient" function.
func (client *Client) Close() (err error) {
	// defer func() {
	// 	if e := recover(); e != nil {
	// 		err = fmt.Errorf("%v", e)
	// 	}
	// }()
	client.wasm.Clear(client.api)
	client.wasm.Free(client.api)
	if client.pixImage != 0 {
		client.wasm.DestroyPixImage(client.pixImage)
		client.pixImage = 0
	}
	client.wasm.Close()
	return err
}

// Version provides the version of Tesseract used by this client.
func (client *Client) Version() string {
	version := client.wasm.Version(client.api)[0]
	return client.wasm.ReadString(version)
}

// SetImage sets path to image file to be processed OCR.
func (client *Client) SetImage(imagepath string) error {

	if client.api == 0 {
		return fmt.Errorf("TessBaseAPI is not constructed, please use `gosseract.NewClient`")
	}
	if imagepath == "" {
		return fmt.Errorf("image path cannot be empty")
	}
	if _, err := os.Stat(imagepath); err != nil {
		return fmt.Errorf("cannot detect the stat of specified file: %v", err)
	}

	imagepath, _ = filepath.Abs(imagepath)

	if client.pixImage != 0 {
		client.wasm.DestroyPixImage(client.pixImage)
		client.pixImage = 0
	}

	imagepathPtr := client.wasm.malloc(uint64(len(imagepath) + 1))[0]
	defer client.wasm.free(imagepathPtr)
	client.wasm.module.Memory().Write(uint32(imagepathPtr), append([]byte(imagepath), 0))

	img := client.wasm.CreatePixImageByFilepath(imagepathPtr)[0]
	client.pixImage = img

	return nil
}

// SetImageFromBytes sets the image data to be processed OCR.
func (client *Client) SetImageFromBytes(data []byte) error {

	if client.api == 0 {
		return fmt.Errorf("TessBaseAPI is not constructed, please use `gosseract.NewClient`")
	}
	if len(data) == 0 {
		return fmt.Errorf("image data cannot be empty")
	}

	if client.pixImage != 0 {
		client.wasm.DestroyPixImage(client.pixImage)
		client.pixImage = 0
	}

	imagePtr := client.wasm.malloc(uint64(len(data)))[0]
	client.wasm.module.Memory().Write(uint32(imagePtr), data)

	img := client.wasm.CreatePixImageFromBytes(imagePtr, uint64(len(data)))[0]
	client.pixImage = img

	return nil
}

// SetLanguage sets languages to use. English as default.
func (client *Client) SetLanguage(langs ...string) error {
	if len(langs) == 0 {
		return fmt.Errorf("languages cannot be empty")
	}

	client.Languages = langs

	client.flagForInit()

	return nil
}

// DisableOutput ...
func (client *Client) DisableOutput() error {
	err := client.SetVariable(DEBUG_FILE, os.DevNull)

	client.setVariablesToInitializedAPIIfNeeded()

	return err
}

// SetWhitelist sets whitelist chars.
// See official documentation for whitelist here https://tesseract-ocr.github.io/tessdoc/ImproveQuality#dictionaries-word-lists-and-patterns
func (client *Client) SetWhitelist(whitelist string) error {
	err := client.SetVariable(TESSEDIT_CHAR_WHITELIST, whitelist)

	client.setVariablesToInitializedAPIIfNeeded()

	return err
}

// SetBlacklist sets blacklist chars.
// See official documentation for blacklist here https://tesseract-ocr.github.io/tessdoc/ImproveQuality#dictionaries-word-lists-and-patterns
func (client *Client) SetBlacklist(blacklist string) error {
	err := client.SetVariable(TESSEDIT_CHAR_BLACKLIST, blacklist)

	client.setVariablesToInitializedAPIIfNeeded()

	return err
}

// SetVariable sets parameters, representing tesseract::TessBaseAPI->SetVariable.
// See official documentation here https://zdenop.github.io/tesseract-doc/classtesseract_1_1_tess_base_a_p_i.html#a2e09259c558c6d8e0f7e523cbaf5adf5
// Because `api->SetVariable` must be called after `api->Init`, this method cannot detect unexpected key for variables.
// Check `client.setVariablesToInitializedAPI` for more information.
func (client *Client) SetVariable(key SettableVariable, value string) error {
	client.Variables[key] = value

	client.setVariablesToInitializedAPIIfNeeded()

	return nil
}

// SetPageSegMode sets "Page Segmentation Mode" (PSM) to detect layout of characters.
// See official documentation for PSM here https://tesseract-ocr.github.io/tessdoc/ImproveQuality#page-segmentation-method
// See https://github.com/otiai10/gosseract/issues/52 for more information.
func (client *Client) SetPageSegMode(mode PageSegMode) error {
	client.wasm.SetPageSegMode(client.api, uint64(mode))
	return nil
}

// SetConfigFile sets the file path to config file.
func (client *Client) SetConfigFile(fpath string) error {
	info, err := os.Stat(fpath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("the specified config file path seems to be a directory")
	}
	client.ConfigFilePath, err = filepath.Abs(fpath)
	if err != nil {
		return err
	}

	client.flagForInit()

	return nil
}

// SetTessdataPrefix sets path to the models directory.
// Environment variable TESSDATA_PREFIX is used as default.
func (client *Client) SetTessdataPrefix(prefix string) error {
	if prefix == "" {
		return fmt.Errorf("tessdata prefix could not be empty")
	}
	client.TessdataPrefix, _ = filepath.Abs(prefix)
	client.flagForInit()
	return nil
}

func (client *Client) setTessdataPrefixWithFS(prefix string, fs fs.FS) {
}

// Initialize tesseract::TessBaseAPI
func (client *Client) init() error {

	if !client.shouldInit {
		client.wasm.SetPixImage(client.api, client.pixImage)
		return nil
	}

	var languages string
	if len(client.Languages) != 0 {
		languages = strings.Join(client.Languages, "+")
	}
	languagesPtr := client.wasm.malloc(uint64(len(languages)) + 1)[0]
	client.wasm.module.Memory().Write(uint32(languagesPtr), append([]byte(languages), 0))
	defer client.wasm.free(languagesPtr)

	var configFilePtr uint64
	if client.ConfigFilePath != "" {
		configFilePtr = client.wasm.malloc(uint64(len(client.ConfigFilePath) + 1))[0]
		println(client.ConfigFilePath)
		client.wasm.module.Memory().Write(uint32(configFilePtr), append([]byte(client.ConfigFilePath), 0))
	}

	var tessdataPrefix string
	if client.TessdataPrefix != "" {
		tessdataPrefix = client.TessdataPrefix
	} else {
		tessdataPrefix = "/tessdata/"
	}
	tessdataPrefixPtr := client.wasm.malloc(uint64(len(tessdataPrefix) + 1))[0]

	client.wasm.module.Memory().Write(uint32(tessdataPrefixPtr), append([]byte(tessdataPrefix), 0))
	defer client.wasm.free(tessdataPrefixPtr)

	res := client.wasm.Init(client.api, tessdataPrefixPtr, languagesPtr, configFilePtr, 0)[0]

	if res != 0 {
		return fmt.Errorf("failed to initialize TessBaseAPI with code %d", -1)
	}

	if err := client.setVariablesToInitializedAPI(); err != nil {
		return err
	}

	if client.pixImage == 0 {
		return fmt.Errorf("PixImage is not set, use SetImage or SetImageFromBytes before Text or HOCRText")
	}

	client.wasm.SetPixImage(client.api, client.pixImage)

	client.shouldInit = false

	return nil
}

// This method flag the current instance to be initialized again on the next call to a function that
// requires a gosseract API initialized: when user change the config file or the languages
// the instance needs to init a new gosseract api
func (client *Client) flagForInit() {
	client.shouldInit = true
}

// This method sets all the sspecified variables to TessBaseAPI structure.
// Because `api->SetVariable` must be called after `api->Init()`,
// gosseract.Client.SetVariable cannot call `api->SetVariable` directly.
// See https://zdenop.github.io/tesseract-doc/classtesseract_1_1_tess_base_a_p_i.html#a2e09259c558c6d8e0f7e523cbaf5adf5
func (client *Client) setVariablesToInitializedAPI() error {
	for key, value := range client.Variables {
		keyPtr := client.wasm.malloc(uint64(len(key) + 1))[0]
		valPtr := client.wasm.malloc(uint64(len(key) + 1))[0]
		client.wasm.module.Memory().Write(uint32(keyPtr), append([]byte(string(key)), 0))
		client.wasm.module.Memory().Write(uint32(valPtr), append([]byte(string(value)), 0))
		defer client.wasm.free(keyPtr)
		defer client.wasm.free(valPtr)
		res := client.wasm.SetVariable(client.api, keyPtr, valPtr)[0]
		if res == 0 {
			return fmt.Errorf("failed to set variable with key(%v) and value(%v)", key, value)
		}
	}
	return nil
}

// Call setVariablesToInitializedAPI only if the API is initialized
// it is useful to call when changing variables that does not requires
// to init a new tesseract instance. Otherwise it is better to just flag
// the instance for re-init (Client.flagForInit())
func (client *Client) setVariablesToInitializedAPIIfNeeded() error {
	if !client.shouldInit {
		return client.setVariablesToInitializedAPI()
	}

	return nil
}

// Text finally initialize tesseract::TessBaseAPI, execute OCR and extract text detected as string.
func (client *Client) Text() (out string, err error) {
	if err = client.init(); err != nil {
		return
	}
	resultPtr := client.wasm.Utf8Text(client.api)[0]
	out = client.wasm.ReadString(resultPtr)
	if client.Trim {
		out = strings.Trim(out, "\n")
	}
	return out, err
}

// HOCRText finally initialize tesseract::TessBaseAPI, execute OCR and returns hOCR text.
// See https://en.wikipedia.org/wiki/HOCR for more information of hOCR.
func (client *Client) HOCRText() (out string, err error) {
	if err = client.init(); err != nil {
		return
	}
	textPtr := client.wasm.HocrText(client.api)[0]
	defer client.wasm.free(textPtr)
	out = client.wasm.ReadString(textPtr)
	return
}

// BoundingBox contains the position, confidence and UTF8 text of the recognized word
type BoundingBox struct {
	Box                                image.Rectangle
	Word                               string
	Confidence                         float64
	BlockNum, ParNum, LineNum, WordNum int
}

// GetBoundingBoxes returns bounding boxes for each matched word
func (client *Client) GetBoundingBoxes(level PageIteratorLevel) (out []BoundingBox, err error) {
	if client.api == 0 {
		return out, fmt.Errorf("TessBaseAPI is not constructed, please use `gosseract.NewClient`")
	}
	if err = client.init(); err != nil {
		return
	}
	boundingBoxesPtr := client.wasm.GetBoundingBoxes(client.api, uint64(level))[0]
	defer client.wasm.free(boundingBoxesPtr)
	length, _ := client.wasm.module.Memory().ReadUint32Le(uint32(boundingBoxesPtr))
	boxArrayPtr, _ := client.wasm.module.Memory().ReadUint64Le(uint32(boundingBoxesPtr) + 4)
	defer client.wasm.free(boxArrayPtr)

	readInt := func(base uint64, offset int) int {
		x, _ := client.wasm.module.Memory().ReadUint32Le(uint32(base) + uint32(offset))
		return int(x)
	}

	readFloat64 := func(base uint64, offset int) float64 {
		x, _ := client.wasm.module.Memory().ReadFloat64Le(uint32(base) + uint32(offset))
		return x
	}

	out = make([]BoundingBox, 0, length)

	for i := 0; i < int(length); i++ {
		x1 := readInt(boxArrayPtr, 48*i)
		y1 := readInt(boxArrayPtr, 48*i+4)
		x2 := readInt(boxArrayPtr, 48*i+8)
		y2 := readInt(boxArrayPtr, 48*i+12)
		wordPtr, _ := client.wasm.module.Memory().ReadUint32Le(uint32(boxArrayPtr) + uint32(48*i+16))
		word := client.wasm.ReadString(uint64(wordPtr))
		confidence := readFloat64(boxArrayPtr, 48*i+24)
		out = append(out, BoundingBox{
			Box:        image.Rect(x1, y1, x2, y2),
			Word:       word,
			Confidence: confidence,
		})
	}

	return
}

// GetAvailableLanguages returns a list of available languages in the default tesspath
func GetAvailableLanguages() ([]string, error) {
	languages, err := filepath.Glob(filepath.Join(getDataPath(), "*.traineddata"))
	if err != nil {
		return languages, err
	}
	for i := 0; i < len(languages); i++ {
		languages[i] = filepath.Base(languages[i])
		idx := strings.Index(languages[i], ".")
		languages[i] = languages[i][:idx]
	}
	return languages, nil
}

// GetBoundingBoxesVerbose returns bounding boxes at word level with block_num, par_num, line_num and word_num
// according to the c++ api that returns a formatted TSV output. Reference: `TessBaseAPI::GetTSVText`.
func (client *Client) GetBoundingBoxesVerbose() (out []BoundingBox, err error) {
	if client.api == 0 {
		return out, fmt.Errorf("TessBaseAPI is not constructed, please use `gosseract.NewClient`")
	}
	if err = client.init(); err != nil {
		return
	}
	boundingBoxesPtr := client.wasm.GetBoundingBoxesVerbose(client.api)[0]
	defer client.wasm.free(boundingBoxesPtr)
	length, _ := client.wasm.module.Memory().ReadUint32Le(uint32(boundingBoxesPtr))
	boxArrayPtr, _ := client.wasm.module.Memory().ReadUint64Le(uint32(boundingBoxesPtr) + 4)
	defer client.wasm.free(boxArrayPtr)

	readInt := func(base uint64, offset int) int {
		x, _ := client.wasm.module.Memory().ReadUint32Le(uint32(base) + uint32(offset))
		return int(x)
	}

	readFloat64 := func(base uint64, offset int) float64 {
		x, _ := client.wasm.module.Memory().ReadFloat64Le(uint32(base) + uint32(offset))
		return x
	}

	out = make([]BoundingBox, 0, length)

	for i := 0; i < int(length); i++ {
		x1 := readInt(boxArrayPtr, 48*i)
		y1 := readInt(boxArrayPtr, 48*i+4)
		x2 := readInt(boxArrayPtr, 48*i+8)
		y2 := readInt(boxArrayPtr, 48*i+12)
		wordPtr, _ := client.wasm.module.Memory().ReadUint32Le(uint32(boxArrayPtr) + uint32(48*i+16))
		word := client.wasm.ReadString(uint64(wordPtr))
		confidence := readFloat64(boxArrayPtr, 48*i+24)
		blockNum := readInt(boxArrayPtr, 48*i+32)
		parNum := readInt(boxArrayPtr, 48*i+36)
		lineNum := readInt(boxArrayPtr, 48*i+40)
		wordNum := readInt(boxArrayPtr, 48*i+44)

		out = append(out, BoundingBox{
			Box:        image.Rect(x1, y1, x2, y2),
			Word:       word,
			Confidence: confidence,
			BlockNum:   blockNum,
			ParNum:     parNum,
			LineNum:    lineNum,
			WordNum:    wordNum,
		})
	}

	return
}

// getDataPath is useful hepler to determine where current tesseract
// installation stores trained models
func getDataPath() string {
	wasm := newApi()
	dataPath := wasm.ReadString(wasm.GetDataPath()[0])
	wasm.Close()
	return dataPath
}
