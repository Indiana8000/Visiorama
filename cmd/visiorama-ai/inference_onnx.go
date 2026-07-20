//go:build cgo

package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"os"
	"sort"
	"sync"

	"github.com/disintegration/imaging"
	ort "github.com/yalue/onnxruntime_go"

	"github.com/Indiana8000/visiorama/internal/ai"
	"github.com/Indiana8000/visiorama/internal/convert"
)

// YOLOv8n constants — model input is 640x640, output shape [1, 84, 8400].
// 84 = 4 (cx,cy,w,h) + 80 COCO classes; 8400 = 80×80 + 40×40 + 20×20 anchors.
const (
	yoloW       = 640
	yoloH       = 640
	yoloClasses = 80
	yoloAnchors = 8400
	yoloConfThr = float32(0.25)
	yoloNMSThr  = float32(0.45)
)

// cocoClasses maps 80-class COCO index → label string.
var cocoClasses = [yoloClasses]string{
	"person", "bicycle", "car", "motorcycle", "airplane", "bus", "train", "truck",
	"boat", "traffic light", "fire hydrant", "stop sign", "parking meter", "bench",
	"bird", "cat", "dog", "horse", "sheep", "cow", "elephant", "bear", "zebra",
	"giraffe", "backpack", "umbrella", "handbag", "tie", "suitcase", "frisbee",
	"skis", "snowboard", "sports ball", "kite", "baseball bat", "baseball glove",
	"skateboard", "surfboard", "tennis racket", "bottle", "wine glass", "cup",
	"fork", "knife", "spoon", "bowl", "banana", "apple", "sandwich", "orange",
	"broccoli", "carrot", "hot dog", "pizza", "donut", "cake", "chair", "couch",
	"potted plant", "bed", "dining table", "toilet", "tv", "laptop", "mouse",
	"remote", "keyboard", "cell phone", "microwave", "oven", "toaster", "sink",
	"refrigerator", "book", "clock", "vase", "scissors", "teddy bear",
	"hair drier", "toothbrush",
}

// ortOnce initialises the onnxruntime environment exactly once per process.
var (
	ortOnce sync.Once
	ortErr  error
)

func initORT() error {
	ortOnce.Do(func() {
		// Allow overriding the ORT shared library path via environment variable.
		// Default: "onnxruntime.so" (Linux) / "onnxruntime.dll" (Windows).
		if p := os.Getenv("ORT_LIB_PATH"); p != "" {
			ort.SetSharedLibraryPath(p)
		}
		ortErr = ort.InitializeEnvironment()
	})
	return ortErr
}

// newSessionOptions returns SessionOptions with explicit thread counts so ORT
// does not attempt pthread_setaffinity_np (error code 22 on some kernels).
func newSessionOptions() (*ort.SessionOptions, error) {
	opts, err := ort.NewSessionOptions()
	if err != nil {
		return nil, err
	}
	if err := opts.SetIntraOpNumThreads(1); err != nil {
		opts.Destroy()
		return nil, err
	}
	if err := opts.SetInterOpNumThreads(1); err != nil {
		opts.Destroy()
		return nil, err
	}
	return opts, nil
}

// yoloInstance holds a loaded session and pre-allocated tensor buffers.
// Access is serialised by mu so the same buffers can be reused across calls.
type yoloInstance struct {
	mu           sync.Mutex
	session      *ort.AdvancedSession
	inputData    []float32 // backing slice for inputTensor  [1,3,H,W]
	outputData   []float32 // backing slice for outputTensor [1,84,8400]
	inputTensor  *ort.Tensor[float32]
	outputTensor *ort.Tensor[float32]
}

var (
	yoloInstMu  sync.Mutex
	yoloInst    *yoloInstance
	yoloInstKey string
)

func getYOLOInstance(modelPath string) (*yoloInstance, error) {
	yoloInstMu.Lock()
	defer yoloInstMu.Unlock()

	if yoloInst != nil && yoloInstKey == modelPath {
		return yoloInst, nil
	}

	if err := initORT(); err != nil {
		return nil, fmt.Errorf("onnxruntime init: %w", err)
	}

	inputData := make([]float32, 1*3*yoloH*yoloW)
	outputData := make([]float32, 1*(yoloClasses+4)*yoloAnchors)

	inTensor, err := ort.NewTensor(ort.NewShape(1, 3, yoloH, yoloW), inputData)
	if err != nil {
		return nil, fmt.Errorf("create input tensor: %w", err)
	}

	outTensor, err := ort.NewTensor(ort.NewShape(1, yoloClasses+4, yoloAnchors), outputData)
	if err != nil {
		_ = inTensor.Destroy()
		return nil, fmt.Errorf("create output tensor: %w", err)
	}

	opts, err := newSessionOptions()
	if err != nil {
		_ = inTensor.Destroy()
		_ = outTensor.Destroy()
		return nil, fmt.Errorf("yolo session options: %w", err)
	}
	defer opts.Destroy()

	session, err := ort.NewAdvancedSession(
		modelPath,
		[]string{"images"},
		[]string{"output0"},
		[]ort.Value{inTensor},
		[]ort.Value{outTensor},
		opts,
	)
	if err != nil {
		_ = inTensor.Destroy()
		_ = outTensor.Destroy()
		return nil, fmt.Errorf("load yolov8n session: %w", err)
	}

	inst := &yoloInstance{
		session:      session,
		inputData:    inputData,
		outputData:   outputData,
		inputTensor:  inTensor,
		outputTensor: outTensor,
	}
	yoloInst = inst
	yoloInstKey = modelPath
	return inst, nil
}

// runYOLO runs YOLOv8n object detection on the given image file.
// Returns detected labels with bounding boxes and confidence scores.
func runYOLO(ctx context.Context, modelPath, imagePath string) ([]ai.Label, error) {
	if !fileExists(modelPath) {
		return nil, fmt.Errorf("model not found: %s", modelPath)
	}
	if !fileExists(imagePath) {
		return nil, fmt.Errorf("image not found: %s", imagePath)
	}

	inst, err := getYOLOInstance(modelPath)
	if err != nil {
		return nil, err
	}

	img, err := convert.OpenImage(imagePath)
	if err != nil {
		return nil, fmt.Errorf("open image: %w", err)
	}

	origW := img.Bounds().Dx()
	origH := img.Bounds().Dy()

	inst.mu.Lock()
	defer inst.mu.Unlock()

	scale, padX, padY := letterboxPreprocess(img, inst.inputData)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if err := inst.session.Run(); err != nil {
		return nil, fmt.Errorf("yolo inference: %w", err)
	}

	dets := decodeYOLO(inst.outputData, scale, padX, padY, origW, origH, yoloConfThr)
	dets = nms(dets, yoloNMSThr)

	labels := make([]ai.Label, 0, len(dets))
	for _, d := range dets {
		labels = append(labels, ai.Label{
			Label:      cocoClasses[d.classIdx],
			Confidence: float64(d.score),
			Source:     "yolo",
			BBox: &ai.BBox{
				X: int(d.x1),
				Y: int(d.y1),
				W: int(d.x2 - d.x1),
				H: int(d.y2 - d.y1),
			},
		})
	}
	return labels, nil
}

// letterboxPreprocess resizes img into a 640×640 letterbox and writes the
// CHW float32 tensor (values in [0,1]) into dst.
// Returns the uniform scale and the (x,y) padding added to each side.
func letterboxPreprocess(img image.Image, dst []float32) (scale, padX, padY float32) {
	origW := img.Bounds().Dx()
	origH := img.Bounds().Dy()

	scaleX := float32(yoloW) / float32(origW)
	scaleY := float32(yoloH) / float32(origH)
	scale = scaleX
	if scaleY < scaleX {
		scale = scaleY
	}

	newW := int(float32(origW)*scale + 0.5)
	newH := int(float32(origH)*scale + 0.5)
	padX = float32(yoloW-newW) / 2
	padY = float32(yoloH-newH) / 2

	resized := imaging.Resize(img, newW, newH, imaging.Linear)
	nrgba := imaging.New(yoloW, yoloH, color.NRGBA{128, 128, 128, 255})
	nrgba = imaging.Paste(nrgba, resized, image.Pt(int(padX+0.5), int(padY+0.5)))

	// Write CHW layout into dst: channel * H * W + y * W + x
	for y := 0; y < yoloH; y++ {
		for x := 0; x < yoloW; x++ {
			c := nrgba.NRGBAAt(x, y)
			off := y*yoloW + x
			dst[0*yoloH*yoloW+off] = float32(c.R) / 255.0
			dst[1*yoloH*yoloW+off] = float32(c.G) / 255.0
			dst[2*yoloH*yoloW+off] = float32(c.B) / 255.0
		}
	}
	return
}

// det is an intermediate detection before NMS.
type det struct {
	classIdx    int
	score       float32
	x1, y1, x2, y2 float32
}

// decodeYOLO parses the [1,84,8400] output tensor and returns detections
// with coordinates mapped back to original image space.
func decodeYOLO(output []float32, scale, padX, padY float32, origW, origH int, confThr float32) []det {
	var dets []det

	for i := 0; i < yoloAnchors; i++ {
		cx := output[0*yoloAnchors+i]
		cy := output[1*yoloAnchors+i]
		w := output[2*yoloAnchors+i]
		h := output[3*yoloAnchors+i]

		maxScore := float32(0)
		maxClass := 0
		for c := 0; c < yoloClasses; c++ {
			s := output[(4+c)*yoloAnchors+i]
			if s > maxScore {
				maxScore = s
				maxClass = c
			}
		}

		if maxScore < confThr {
			continue
		}

		// Remove letterbox padding and undo scale to get original-image coords.
		x1 := (cx - w/2 - padX) / scale
		y1 := (cy - h/2 - padY) / scale
		x2 := (cx + w/2 - padX) / scale
		y2 := (cy + h/2 - padY) / scale

		x1 = clamp32(x1, 0, float32(origW))
		y1 = clamp32(y1, 0, float32(origH))
		x2 = clamp32(x2, 0, float32(origW))
		y2 = clamp32(y2, 0, float32(origH))

		if x2-x1 < 1 || y2-y1 < 1 {
			continue
		}

		dets = append(dets, det{classIdx: maxClass, score: maxScore, x1: x1, y1: y1, x2: x2, y2: y2})
	}
	return dets
}

// nms applies per-class non-maximum suppression (greedy, score-descending).
func nms(dets []det, iouThr float32) []det {
	sort.Slice(dets, func(i, j int) bool { return dets[i].score > dets[j].score })

	keep := make([]bool, len(dets))
	for i := range keep {
		keep[i] = true
	}
	for i := 0; i < len(dets); i++ {
		if !keep[i] {
			continue
		}
		for j := i + 1; j < len(dets); j++ {
			if !keep[j] {
				continue
			}
			if dets[i].classIdx == dets[j].classIdx && iou(dets[i], dets[j]) > iouThr {
				keep[j] = false
			}
		}
	}

	out := dets[:0]
	for i, d := range dets {
		if keep[i] {
			out = append(out, d)
		}
	}
	return out
}

func iou(a, b det) float32 {
	ix1 := max(a.x1, b.x1)
	iy1 := max(a.y1, b.y1)
	ix2 := min(a.x2, b.x2)
	iy2 := min(a.y2, b.y2)
	iw := max(float32(0), ix2-ix1)
	ih := max(float32(0), iy2-iy1)
	inter := iw * ih
	union := (a.x2-a.x1)*(a.y2-a.y1) + (b.x2-b.x1)*(b.y2-b.y1) - inter
	if union <= 0 {
		return 0
	}
	return inter / union
}

func clamp32(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

