//go:build cgo

package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/disintegration/imaging"
	ort "github.com/yalue/onnxruntime_go"

	"github.com/Indiana8000/visiorama/internal/ai"
)

// ── SCRFD constants ────────────────────────────────────────────────────────────
// Input: [1,3,640,640]  normalization: (px - 127.5) / 128.0  RGB CHW
// 9 outputs: scores×3 + bboxes×3 + keypoints×3  (strides 8,16,32)

const (
	scrfdW         = 640
	scrfdH         = 640
	scrfdConfThr   = float32(0.5)
	scrfdNMSThr    = float32(0.4)
	arcfaceSize    = 112 // ArcFace crop side length
	arcfaceEmbedDim = 512
)

// strides and anchor counts per stride for SCRFD-10G
var scrfdStrides = [3]int{8, 16, 32}

// scrfdOutputNames in order returned by the model.
// Confirmed via onnxruntime inspect on scrfd_10g_bnkps.onnx.
var scrfdOutputNames = []string{
	"448", "471", "494", // scores  stride 8/16/32
	"451", "474", "497", // bboxes  stride 8/16/32
	"454", "477", "500", // keypoints stride 8/16/32
}

// ── ArcFace input/output names ─────────────────────────────────────────────────
const (
	arcfaceInputName  = "input.1"
	arcfaceOutputName = "1333"
)

// ── Shared session cache ───────────────────────────────────────────────────────

type faceInstances struct {
	mu       sync.Mutex
	scrfd    *scrfdInstance
	arcface  *arcfaceInstance
	scrfdKey string
	arcKey   string
}

var faceInst faceInstances

type scrfdInstance struct {
	mu          sync.Mutex
	session     *ort.AdvancedSession
	inputData   []float32
	inputTensor *ort.Tensor[float32]
	// 9 output tensors, pre-allocated
	outScores [3]*ort.Tensor[float32]
	outBoxes  [3]*ort.Tensor[float32]
	outKps    [3]*ort.Tensor[float32]
}

type arcfaceInstance struct {
	mu           sync.Mutex
	session      *ort.AdvancedSession
	inputData    []float32
	inputTensor  *ort.Tensor[float32]
	outputData   []float32
	outputTensor *ort.Tensor[float32]
}

// anchorCounts returns number of anchors for each stride level.
// SCRFD-10G uses 2 anchors per cell.
func anchorCount(stride int) int {
	cells := (scrfdH / stride) * (scrfdW / stride)
	return cells * 2
}

func getSCRFDInstance(modelPath string) (*scrfdInstance, error) {
	faceInst.mu.Lock()
	defer faceInst.mu.Unlock()
	if faceInst.scrfd != nil && faceInst.scrfdKey == modelPath {
		return faceInst.scrfd, nil
	}
	if err := initORT(); err != nil {
		return nil, fmt.Errorf("ort init: %w", err)
	}

	inputData := make([]float32, 1*3*scrfdH*scrfdW)
	inTensor, err := ort.NewTensor(ort.NewShape(1, 3, scrfdH, scrfdW), inputData)
	if err != nil {
		return nil, fmt.Errorf("scrfd input tensor: %w", err)
	}

	inst := &scrfdInstance{inputData: inputData, inputTensor: inTensor}

	// Build pre-allocated output tensors.
	// Order: score s8/16/32, box s8/16/32, kps s8/16/32
	inputs := []ort.Value{inTensor}
	outputs := make([]ort.Value, 9)
	for i, stride := range scrfdStrides {
		n := int64(anchorCount(stride))
		sd, _ := ort.NewTensor(ort.NewShape(n, 1), make([]float32, n))
		bd, _ := ort.NewTensor(ort.NewShape(n, 4), make([]float32, n*4))
		kd, _ := ort.NewTensor(ort.NewShape(n, 10), make([]float32, n*10))
		inst.outScores[i] = sd
		inst.outBoxes[i] = bd
		inst.outKps[i] = kd
		outputs[i] = sd
		outputs[3+i] = bd
		outputs[6+i] = kd
	}

	session, err := ort.NewAdvancedSession(
		modelPath,
		[]string{"input.1"},
		scrfdOutputNames,
		inputs, outputs, nil,
	)
	if err != nil {
		_ = inTensor.Destroy()
		return nil, fmt.Errorf("scrfd session: %w", err)
	}
	inst.session = session
	faceInst.scrfd = inst
	faceInst.scrfdKey = modelPath
	return inst, nil
}

func getArcfaceInstance(modelPath string) (*arcfaceInstance, error) {
	faceInst.mu.Lock()
	defer faceInst.mu.Unlock()
	if faceInst.arcface != nil && faceInst.arcKey == modelPath {
		return faceInst.arcface, nil
	}
	if err := initORT(); err != nil {
		return nil, fmt.Errorf("ort init: %w", err)
	}

	inputData := make([]float32, 1*3*arcfaceSize*arcfaceSize)
	outputData := make([]float32, arcfaceEmbedDim)
	inTensor, err := ort.NewTensor(ort.NewShape(1, 3, arcfaceSize, arcfaceSize), inputData)
	if err != nil {
		return nil, fmt.Errorf("arcface input tensor: %w", err)
	}
	outTensor, err := ort.NewTensor(ort.NewShape(1, arcfaceEmbedDim), outputData)
	if err != nil {
		_ = inTensor.Destroy()
		return nil, fmt.Errorf("arcface output tensor: %w", err)
	}
	session, err := ort.NewAdvancedSession(
		modelPath,
		[]string{arcfaceInputName},
		[]string{arcfaceOutputName},
		[]ort.Value{inTensor},
		[]ort.Value{outTensor},
		nil,
	)
	if err != nil {
		_ = inTensor.Destroy()
		_ = outTensor.Destroy()
		return nil, fmt.Errorf("arcface session: %w", err)
	}
	inst := &arcfaceInstance{
		session:      session,
		inputData:    inputData,
		inputTensor:  inTensor,
		outputData:   outputData,
		outputTensor: outTensor,
	}
	faceInst.arcface = inst
	faceInst.arcKey = modelPath
	return inst, nil
}

// ── Face detection pipeline ────────────────────────────────────────────────────

type faceDetection struct {
	x1, y1, x2, y2 float32
	score           float32
	kps             [5][2]float32 // 5 landmarks (x,y) in original image coords
}

// runFacePipeline replaces the I-3 stub.
// Detects faces with SCRFD, aligns crops, embeds with ArcFace.
func runFacePipeline(ctx context.Context, detectorPath, embeddingPath, imagePath string) ([]ai.Face, error) {
	if !fileExists(detectorPath) {
		return nil, fmt.Errorf("detector model not found: %s", detectorPath)
	}
	if !fileExists(embeddingPath) {
		return nil, fmt.Errorf("embedding model not found: %s", embeddingPath)
	}
	if !fileExists(imagePath) {
		return nil, fmt.Errorf("image not found: %s", imagePath)
	}

	scrfdInst, err := getSCRFDInstance(detectorPath)
	if err != nil {
		return nil, err
	}
	arcInst, err := getArcfaceInstance(embeddingPath)
	if err != nil {
		return nil, err
	}

	img, err := imaging.Open(imagePath, imaging.AutoOrientation(true))
	if err != nil {
		return nil, fmt.Errorf("open image: %w", err)
	}
	origW := img.Bounds().Dx()
	origH := img.Bounds().Dy()

	// --- SCRFD detection ---
	scrfdInst.mu.Lock()
	scrfdScale, scrfdPadX, scrfdPadY := scrfdPreprocess(img, scrfdInst.inputData)
	select {
	case <-ctx.Done():
		scrfdInst.mu.Unlock()
		return nil, ctx.Err()
	default:
	}
	if err := scrfdInst.session.Run(); err != nil {
		scrfdInst.mu.Unlock()
		return nil, fmt.Errorf("scrfd inference: %w", err)
	}
	dets := decodeSCRFD(scrfdInst, scrfdScale, scrfdPadX, scrfdPadY, origW, origH)
	scrfdInst.mu.Unlock()

	dets = faceNMS(dets, scrfdNMSThr)

	if len(dets) == 0 {
		return nil, nil
	}

	// --- ArcFace embedding per face ---
	cropDir := filepath.Join(filepath.Dir(embeddingPath), "..", "crops")
	_ = os.MkdirAll(cropDir, 0755)

	var faces []ai.Face
	for _, d := range dets {
		select {
		case <-ctx.Done():
			return faces, ctx.Err()
		default:
		}

		crop := alignFace(img, d.kps, arcfaceSize)

		arcInst.mu.Lock()
		arcfacePreprocess(crop, arcInst.inputData)
		if err := arcInst.session.Run(); err != nil {
			arcInst.mu.Unlock()
			continue
		}
		emb := make([]float32, arcfaceEmbedDim)
		copy(emb, arcInst.outputData)
		arcInst.mu.Unlock()

		l2Normalize(emb)

		cropPath := filepath.Join(cropDir, fmt.Sprintf("face_%d_%d.jpg",
			int(d.x1), int(d.y1)))
		if err := saveCrop(crop, cropPath); err != nil {
			cropPath = ""
		}

		faces = append(faces, ai.Face{
			BBox: ai.BBox{
				X: int(d.x1),
				Y: int(d.y1),
				W: int(d.x2 - d.x1),
				H: int(d.y2 - d.y1),
			},
			Embedding: emb,
			CropPath:  cropPath,
		})
	}
	return faces, nil
}

// ── SCRFD preprocessing ────────────────────────────────────────────────────────

func scrfdPreprocess(img image.Image, dst []float32) (scale, padX, padY float32) {
	origW := img.Bounds().Dx()
	origH := img.Bounds().Dy()
	scaleX := float32(scrfdW) / float32(origW)
	scaleY := float32(scrfdH) / float32(origH)
	scale = scaleX
	if scaleY < scaleX {
		scale = scaleY
	}
	newW := int(float32(origW)*scale + 0.5)
	newH := int(float32(origH)*scale + 0.5)
	padX = float32(scrfdW-newW) / 2
	padY = float32(scrfdH-newH) / 2

	resized := imaging.Resize(img, newW, newH, imaging.Linear)
	canvas := imaging.New(scrfdW, scrfdH, color.NRGBA{0, 0, 0, 255})
	canvas = imaging.Paste(canvas, resized, image.Pt(int(padX+0.5), int(padY+0.5)))

	// (px - 127.5) / 128.0, RGB CHW
	for y := 0; y < scrfdH; y++ {
		for x := 0; x < scrfdW; x++ {
			c := canvas.NRGBAAt(x, y)
			off := y*scrfdW + x
			dst[0*scrfdH*scrfdW+off] = (float32(c.R) - 127.5) / 128.0
			dst[1*scrfdH*scrfdW+off] = (float32(c.G) - 127.5) / 128.0
			dst[2*scrfdH*scrfdW+off] = (float32(c.B) - 127.5) / 128.0
		}
	}
	return
}

// ── SCRFD output decode ────────────────────────────────────────────────────────

func decodeSCRFD(inst *scrfdInstance, scale, padX, padY float32, origW, origH int) []faceDetection {
	var dets []faceDetection

	for si, stride := range scrfdStrides {
		scores := inst.outScores[si].GetData()
		boxes := inst.outBoxes[si].GetData()
		kps := inst.outKps[si].GetData()

		cols := scrfdW / stride
		rows := scrfdH / stride

		anchorIdx := 0
		for r := 0; r < rows; r++ {
			for c := 0; c < cols; c++ {
				for a := 0; a < 2; a++ { // 2 anchors per cell
					score := scores[anchorIdx]
					if score < scrfdConfThr {
						anchorIdx++
						continue
					}

					// Anchor centre in input-image coords
					cx := (float32(c) + 0.5) * float32(stride)
					cy := (float32(r) + 0.5) * float32(stride)

					// Box regression (SCRFD uses dist2bbox: ltrb * stride)
					b := boxes[anchorIdx*4:]
					x1 := cx - b[0]*float32(stride)
					y1 := cy - b[1]*float32(stride)
					x2 := cx + b[2]*float32(stride)
					y2 := cy + b[3]*float32(stride)

					// 5-point keypoints
					var pts [5][2]float32
					kBase := anchorIdx * 10
					for p := 0; p < 5; p++ {
						pts[p][0] = kps[kBase+p*2]*float32(stride) + cx
						pts[p][1] = kps[kBase+p*2+1]*float32(stride) + cy
					}

					// Unpad + unscale to original image coords
					x1 = clamp32((x1-padX)/scale, 0, float32(origW))
					y1 = clamp32((y1-padY)/scale, 0, float32(origH))
					x2 = clamp32((x2-padX)/scale, 0, float32(origW))
					y2 = clamp32((y2-padY)/scale, 0, float32(origH))
					for p := 0; p < 5; p++ {
						pts[p][0] = clamp32((pts[p][0]-padX)/scale, 0, float32(origW))
						pts[p][1] = clamp32((pts[p][1]-padY)/scale, 0, float32(origH))
					}

					if x2-x1 < 1 || y2-y1 < 1 {
						anchorIdx++
						continue
					}

					dets = append(dets, faceDetection{
						x1: x1, y1: y1, x2: x2, y2: y2,
						score: score, kps: pts,
					})
					anchorIdx++
				}
			}
		}
	}
	return dets
}

// ── Face NMS ───────────────────────────────────────────────────────────────────

func faceNMS(dets []faceDetection, iouThr float32) []faceDetection {
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
			if faceIOU(dets[i], dets[j]) > iouThr {
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

func faceIOU(a, b faceDetection) float32 {
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

// ── 5-point affine alignment ───────────────────────────────────────────────────

// arcface112Template is the canonical 112×112 landmark positions from InsightFace.
var arcface112Template = [5][2]float32{
	{38.2946, 51.6963},
	{73.5318, 51.5014},
	{56.0252, 71.7366},
	{41.5493, 92.3655},
	{70.7299, 92.2041},
}

// alignFace produces a 112×112 NRGBA crop aligned to arcface112Template
// using an approximate similarity transform (scale+rotate+translate, no shear).
func alignFace(src image.Image, kps [5][2]float32, size int) *image.NRGBA {
	// Estimate similarity transform from detected kps → template.
	// Uses the least-squares closed-form for similarity (umeyama-lite).
	a, b, tx, ty := similarityTransform(kps, arcface112Template)

	// Scale from 112-template space to desired size.
	sf := float64(size) / 112.0
	a *= float32(sf)
	b *= float32(sf)
	tx *= float32(sf)
	ty *= float32(sf)

	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	srcNRGBA := imaging.Clone(src)

	// Inverse warp: for each dst pixel find src pixel.
	// Forward: dst = A*src + t  where A = [[a,-b],[b,a]]
	// Inverse: src = A^T*(dst - t)
	for dy := 0; dy < size; dy++ {
		for dx := 0; dx < size; dx++ {
			// Apply inverse transform
			fx := float64(a)*(float64(dx)-float64(tx)) + float64(b)*(float64(dy)-float64(ty))
			fy := -float64(b)*(float64(dx)-float64(tx)) + float64(a)*(float64(dy)-float64(ty))
			// Bilinear sample from src
			c := bilinearSample(srcNRGBA, fx, fy)
			dst.SetNRGBA(dx, dy, c)
		}
	}
	return dst
}

// similarityTransform returns (a,b,tx,ty) for the transform
// dst[i] = [[a,-b],[b,a]] * src[i] + [tx,ty]
// using the closed-form umeyama similarity for 5 point pairs.
func similarityTransform(src, dst [5][2]float32) (a, b, tx, ty float32) {
	n := float64(5)
	var mx, my, mdx, mdy float64
	for i := 0; i < 5; i++ {
		mx += float64(src[i][0])
		my += float64(src[i][1])
		mdx += float64(dst[i][0])
		mdy += float64(dst[i][1])
	}
	mx /= n
	my /= n
	mdx /= n
	mdy /= n

	var sigma2 float64
	for i := 0; i < 5; i++ {
		dx := float64(src[i][0]) - mx
		dy := float64(src[i][1]) - my
		sigma2 += dx*dx + dy*dy
	}
	sigma2 /= n

	var sxy, syx float64
	for i := 0; i < 5; i++ {
		dx := float64(src[i][0]) - mx
		dy := float64(src[i][1]) - my
		ddx := float64(dst[i][0]) - mdx
		ddy := float64(dst[i][1]) - mdy
		sxy += dx*ddx + dy*ddy
		syx += dx*ddy - dy*ddx
	}
	sxy /= n
	syx /= n

	if sigma2 < 1e-10 {
		// Degenerate — return identity-ish
		return 1, 0, float32(mdx - mx), float32(mdy - my)
	}

	scale := math.Sqrt(sxy*sxy+syx*syx) / sigma2
	theta := math.Atan2(syx, sxy)
	cosT := math.Cos(theta) * scale
	sinT := math.Sin(theta) * scale

	a = float32(cosT)
	b = float32(sinT)
	tx = float32(mdx - cosT*mx + sinT*my)
	ty = float32(mdy - sinT*mx - cosT*my)
	return
}

func bilinearSample(img *image.NRGBA, x, y float64) color.NRGBA {
	x0 := int(math.Floor(x))
	y0 := int(math.Floor(y))
	x1 := x0 + 1
	y1 := y0 + 1
	fx := float32(x - float64(x0))
	fy := float32(y - float64(y0))
	bounds := img.Bounds()

	clampX := func(v int) int {
		if v < bounds.Min.X {
			return bounds.Min.X
		}
		if v >= bounds.Max.X {
			return bounds.Max.X - 1
		}
		return v
	}
	clampY := func(v int) int {
		if v < bounds.Min.Y {
			return bounds.Min.Y
		}
		if v >= bounds.Max.Y {
			return bounds.Max.Y - 1
		}
		return v
	}

	c00 := img.NRGBAAt(clampX(x0), clampY(y0))
	c10 := img.NRGBAAt(clampX(x1), clampY(y0))
	c01 := img.NRGBAAt(clampX(x0), clampY(y1))
	c11 := img.NRGBAAt(clampX(x1), clampY(y1))

	lerp := func(a, b uint8, t float32) uint8 {
		return uint8(float32(a)*(1-t) + float32(b)*t + 0.5)
	}
	top := color.NRGBA{
		R: lerp(c00.R, c10.R, fx),
		G: lerp(c00.G, c10.G, fx),
		B: lerp(c00.B, c10.B, fx),
		A: 255,
	}
	bot := color.NRGBA{
		R: lerp(c01.R, c11.R, fx),
		G: lerp(c01.G, c11.G, fx),
		B: lerp(c01.B, c11.B, fx),
		A: 255,
	}
	return color.NRGBA{
		R: lerp(top.R, bot.R, fy),
		G: lerp(top.G, bot.G, fy),
		B: lerp(top.B, bot.B, fy),
		A: 255,
	}
}

// ── ArcFace preprocessing ──────────────────────────────────────────────────────

func arcfacePreprocess(crop *image.NRGBA, dst []float32) {
	for y := 0; y < arcfaceSize; y++ {
		for x := 0; x < arcfaceSize; x++ {
			c := crop.NRGBAAt(x, y)
			off := y*arcfaceSize + x
			dst[0*arcfaceSize*arcfaceSize+off] = (float32(c.R) - 127.5) / 128.0
			dst[1*arcfaceSize*arcfaceSize+off] = (float32(c.G) - 127.5) / 128.0
			dst[2*arcfaceSize*arcfaceSize+off] = (float32(c.B) - 127.5) / 128.0
		}
	}
}

// ── Utilities ──────────────────────────────────────────────────────────────────

func l2Normalize(v []float32) {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum < 1e-12 {
		return
	}
	inv := float32(1.0 / math.Sqrt(sum))
	for i := range v {
		v[i] *= inv
	}
}

func saveCrop(img *image.NRGBA, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return jpeg.Encode(f, img, &jpeg.Options{Quality: 90})
}
