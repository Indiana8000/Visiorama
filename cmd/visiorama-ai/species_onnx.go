//go:build cgo

package main

import (
	"context"
	"fmt"
	"image"
	"math"
	"sync"

	"github.com/disintegration/imaging"
	ort "github.com/yalue/onnxruntime_go"

	"github.com/Indiana8000/visiorama/internal/ai"
	"github.com/Indiana8000/visiorama/internal/convert"
)

// MobileNetV3-Small (Qualcomm AI Hub) — ImageNet-1000 classifier.
// Input:  image_tensor [1,3,224,224] float32, values in [0,1] RGB CHW
// Output: class_logits [1,1000] raw logits
// Species detection works by renormalising softmax over a per-class subset of
// ImageNet indices (dog breeds, cat/feline species, bird species).
const (
	speciesW   = 224
	speciesH   = 224
	speciesAll = 1000

	speciesInputName  = "image_tensor"
	speciesOutputName = "class_logits"

	// Minimum renormalised probability within the species subset.
	speciesConfThr = float32(0.10)
)

// dogBreedMap: ~99 dog breed indices from ImageNet-1000 (Qualcomm labels.txt).
var dogBreedMap = map[int]string{
	151: "Chihuahua", 152: "Japanese spaniel", 153: "Maltese dog", 154: "Pekinese",
	155: "Shih-Tzu", 156: "Blenheim spaniel", 157: "papillon", 158: "toy terrier",
	159: "Rhodesian ridgeback", 160: "Afghan hound", 162: "beagle", 163: "bloodhound",
	164: "bluetick", 165: "black-and-tan coonhound", 166: "Walker hound",
	167: "English foxhound", 168: "redbone", 169: "borzoi", 170: "Irish wolfhound",
	171: "Italian greyhound", 172: "whippet", 173: "Ibizan hound",
	174: "Norwegian elkhound", 175: "otterhound", 176: "Saluki",
	177: "Scottish deerhound", 178: "Weimaraner", 179: "Staffordshire bullterrier",
	180: "American Staffordshire terrier", 181: "Bedlington terrier",
	182: "Border terrier", 183: "Kerry blue terrier", 184: "Irish terrier",
	185: "Norfolk terrier", 186: "Norwich terrier", 187: "Yorkshire terrier",
	188: "wire-haired fox terrier", 189: "Lakeland terrier", 190: "Sealyham terrier",
	193: "Australian terrier", 199: "Scotch terrier", 200: "Tibetan terrier",
	201: "silky terrier", 202: "soft-coated wheaten terrier",
	203: "West Highland white terrier", 205: "flat-coated retriever",
	206: "curly-coated retriever", 207: "golden retriever", 208: "Labrador retriever",
	209: "Chesapeake Bay retriever", 210: "German short-haired pointer", 211: "vizsla",
	212: "English setter", 213: "Irish setter", 214: "Gordon setter",
	215: "Brittany spaniel", 218: "Welsh springer spaniel", 219: "cocker spaniel",
	220: "Sussex spaniel", 221: "Irish water spaniel", 222: "kuvasz",
	223: "schipperke", 224: "groenendael", 225: "malinois", 226: "briard",
	227: "kelpie", 228: "komondor", 229: "Old English sheepdog",
	230: "Shetland sheepdog", 231: "collie", 232: "Border collie",
	233: "Bouvier des Flandres", 234: "Rottweiler", 235: "German shepherd",
	236: "Doberman", 237: "miniature pinscher", 238: "Greater Swiss Mountain dog",
	239: "Bernese mountain dog", 242: "boxer", 243: "bull mastiff",
	244: "Tibetan mastiff", 245: "French bulldog", 248: "Eskimo dog",
	249: "malamute", 250: "Siberian husky", 251: "dalmatian", 252: "affenpinscher",
	253: "basenji", 254: "pug", 255: "Leonberg", 256: "Newfoundland",
	258: "Samoyed", 259: "Pomeranian", 261: "keeshond", 265: "toy poodle",
	266: "miniature poodle", 267: "standard poodle", 273: "dingo",
	275: "African hunting dog",
}

// catSpeciesMap: domestic cat breeds + wild felines in ImageNet-1000.
var catSpeciesMap = map[int]string{
	281: "tabby cat", 282: "tiger cat", 283: "Persian cat", 284: "Siamese cat",
	285: "Egyptian cat", 286: "cougar", 287: "lynx", 288: "leopard",
	289: "snow leopard", 290: "jaguar", 291: "lion", 292: "tiger", 293: "cheetah",
}

// birdSpeciesMap: bird species in ImageNet-1000 (false-positives excluded).
var birdSpeciesMap = map[int]string{
	7: "cock", 8: "hen", 9: "ostrich", 10: "brambling", 11: "goldfinch",
	12: "house finch", 14: "indigo bunting", 15: "robin", 16: "bulbul",
	17: "jay", 18: "magpie", 19: "chickadee", 21: "kite", 22: "bald eagle",
	23: "vulture", 24: "great grey owl", 80: "black grouse", 81: "ptarmigan",
	82: "ruffed grouse", 83: "prairie chicken", 84: "peacock", 85: "quail",
	86: "partridge", 87: "junco", 88: "macaw", 89: "sulphur-crested cockatoo",
	90: "lorikeet", 91: "African grey parrot", 94: "hummingbird", 95: "jacamar",
	96: "toucan", 99: "goose", 100: "black swan", 127: "white stork",
	128: "black stork", 129: "spoonbill", 130: "flamingo", 131: "little blue heron",
	132: "American egret", 133: "bittern", 134: "crane", 135: "limpkin",
	136: "European gallinule", 137: "American coot", 138: "bustard",
	139: "ruddy turnstone", 140: "red-backed sandpiper", 141: "redshank",
	142: "dowitcher", 143: "oystercatcher", 144: "pelican", 145: "king penguin",
	146: "albatross",
}

// speciesInstance holds a loaded MobileNetV3 session with pre-allocated buffers.
type speciesInstance struct {
	mu           sync.Mutex
	session      *ort.AdvancedSession
	inputData    []float32
	outputData   []float32
	inputTensor  *ort.Tensor[float32]
	outputTensor *ort.Tensor[float32]
}

var (
	speciesInstMu  sync.Mutex
	speciesInst    *speciesInstance
	speciesInstKey string
)

func getSpeciesInstance(modelPath string) (*speciesInstance, error) {
	speciesInstMu.Lock()
	defer speciesInstMu.Unlock()

	if speciesInst != nil && speciesInstKey == modelPath {
		return speciesInst, nil
	}

	if err := initORT(); err != nil {
		return nil, fmt.Errorf("onnxruntime init: %w", err)
	}

	inputData := make([]float32, 1*3*speciesH*speciesW)
	outputData := make([]float32, speciesAll)

	inTensor, err := ort.NewTensor(ort.NewShape(1, 3, speciesH, speciesW), inputData)
	if err != nil {
		return nil, fmt.Errorf("create species input tensor: %w", err)
	}
	outTensor, err := ort.NewTensor(ort.NewShape(1, speciesAll), outputData)
	if err != nil {
		_ = inTensor.Destroy()
		return nil, fmt.Errorf("create species output tensor: %w", err)
	}
	opts, err := newSessionOptions()
	if err != nil {
		_ = inTensor.Destroy()
		_ = outTensor.Destroy()
		return nil, fmt.Errorf("species session options: %w", err)
	}
	defer opts.Destroy()

	session, err := ort.NewAdvancedSession(
		modelPath,
		[]string{speciesInputName},
		[]string{speciesOutputName},
		[]ort.Value{inTensor},
		[]ort.Value{outTensor},
		opts,
	)
	if err != nil {
		_ = inTensor.Destroy()
		_ = outTensor.Destroy()
		return nil, fmt.Errorf("load species session: %w", err)
	}

	inst := &speciesInstance{
		session:      session,
		inputData:    inputData,
		outputData:   outputData,
		inputTensor:  inTensor,
		outputTensor: outTensor,
	}
	speciesInst = inst
	speciesInstKey = modelPath
	return inst, nil
}

// runSpeciesForLabels opens the image once and classifies the species/breed for
// every label matching yoloClass that has a bounding box.
// classMap maps ImageNet-1000 index → species name.
// Returned labels carry source="species" and the same BBox as the YOLO detection.
func runSpeciesForLabels(ctx context.Context, modelPath, imagePath string, labels []ai.Label, yoloClass string, classMap map[int]string) ([]ai.Label, error) {
	var targets []ai.Label
	for _, l := range labels {
		if l.Label == yoloClass && l.BBox != nil {
			targets = append(targets, l)
		}
	}
	if len(targets) == 0 {
		return nil, nil
	}

	img, err := convert.OpenImage(imagePath)
	if err != nil {
		return nil, fmt.Errorf("open image for species (%s): %w", yoloClass, err)
	}

	var out []ai.Label
	for _, l := range targets {
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		default:
		}
		lbl, err := runSpeciesClassify(ctx, modelPath, img, *l.BBox, classMap)
		if err != nil {
			return out, err
		}
		if lbl != nil {
			lbl.BBox = l.BBox
			out = append(out, *lbl)
		}
	}
	return out, nil
}

// runSpeciesClassify classifies the crop defined by bbox against classMap.
// Returns nil (no error) when confidence is below threshold.
func runSpeciesClassify(ctx context.Context, modelPath string, img image.Image, bbox ai.BBox, classMap map[int]string) (*ai.Label, error) {
	if !fileExists(modelPath) {
		return nil, fmt.Errorf("model not found: %s", modelPath)
	}

	inst, err := getSpeciesInstance(modelPath)
	if err != nil {
		return nil, err
	}

	bounds := img.Bounds()
	x0 := clampInt(bbox.X, bounds.Min.X, bounds.Max.X)
	y0 := clampInt(bbox.Y, bounds.Min.Y, bounds.Max.Y)
	x1 := clampInt(bbox.X+bbox.W, bounds.Min.X, bounds.Max.X)
	y1 := clampInt(bbox.Y+bbox.H, bounds.Min.Y, bounds.Max.Y)
	if x1-x0 < 1 || y1-y0 < 1 {
		return nil, nil
	}

	crop := imaging.Crop(img, image.Rect(x0, y0, x1, y1))
	resized := imaging.Resize(crop, speciesW, speciesH, imaging.Linear)

	inst.mu.Lock()
	defer inst.mu.Unlock()

	speciesPreprocess(resized, inst.inputData)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if err := inst.session.Run(); err != nil {
		return nil, fmt.Errorf("species inference: %w", err)
	}

	name, prob := speciesTopClass(inst.outputData, classMap)
	if prob < speciesConfThr {
		return nil, nil
	}

	return &ai.Label{
		Label:      name,
		Confidence: float64(prob),
		Source:     "species",
	}, nil
}

// speciesTopClass runs softmax over all 1000 logits, renormalises over the
// provided class-index subset, and returns the top name and probability.
func speciesTopClass(logits []float32, classMap map[int]string) (string, float32) {
	maxV := logits[0]
	for _, v := range logits[1:] {
		if v > maxV {
			maxV = v
		}
	}
	probs := make([]float32, speciesAll)
	for i, v := range logits {
		probs[i] = float32(math.Exp(float64(v - maxV)))
	}

	var subsetSum float32
	for idx := range classMap {
		subsetSum += probs[idx]
	}
	if subsetSum < 1e-12 {
		return "", 0
	}

	bestIdx, bestProb := 0, float32(0)
	for idx := range classMap {
		p := probs[idx] / subsetSum
		if p > bestProb {
			bestProb = p
			bestIdx = idx
		}
	}
	return classMap[bestIdx], bestProb
}

// speciesPreprocess converts a resized NRGBA image to CHW float32 scaled to [0,1].
// Qualcomm AI Hub MobileNetV3-Small expects value_range [0,1].
func speciesPreprocess(img *image.NRGBA, dst []float32) {
	for y := 0; y < speciesH; y++ {
		for x := 0; x < speciesW; x++ {
			c := img.NRGBAAt(x, y)
			off := y*speciesW + x
			dst[0*speciesH*speciesW+off] = float32(c.R) / 255.0
			dst[1*speciesH*speciesW+off] = float32(c.G) / 255.0
			dst[2*speciesH*speciesW+off] = float32(c.B) / 255.0
		}
	}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
