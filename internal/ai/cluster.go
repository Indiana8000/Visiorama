package ai

import "math"

// DBSCANResult maps face ID → cluster ID (-1 = noise/unassigned).
type DBSCANResult map[int64]int

// ClusterFaces runs DBSCAN on face embeddings.
// eps: cosine-distance threshold (try 0.4); minPts: minimum cluster size (try 2).
// Embeddings must be L2-normalised — cosine distance = 1 - dot product.
func ClusterFaces(faces []FaceEmbedding, eps float64, minPts int) DBSCANResult {
	n := len(faces)
	result := make(DBSCANResult, n)
	for _, f := range faces {
		result[f.ID] = -1
	}

	visited := make([]bool, n)
	clusterID := 0

	for i := 0; i < n; i++ {
		if visited[i] {
			continue
		}
		visited[i] = true
		neighbours := regionQuery(faces, i, eps)
		if len(neighbours) < minPts {
			continue // noise for now — may be absorbed later
		}
		expandCluster(faces, result, visited, i, neighbours, clusterID, eps, minPts)
		clusterID++
	}
	return result
}

// FaceEmbedding pairs a face DB id with its L2-normalised 512d embedding.
type FaceEmbedding struct {
	ID        int64
	Embedding []float32
}

func expandCluster(faces []FaceEmbedding, result DBSCANResult, visited []bool,
	idx int, neighbours []int, clusterID int, eps float64, minPts int) {
	result[faces[idx].ID] = clusterID
	i := 0
	for i < len(neighbours) {
		nb := neighbours[i]
		if !visited[nb] {
			visited[nb] = true
			nb2 := regionQuery(faces, nb, eps)
			if len(nb2) >= minPts {
				neighbours = append(neighbours, nb2...)
			}
		}
		if result[faces[nb].ID] == -1 {
			result[faces[nb].ID] = clusterID
		}
		i++
	}
}

func regionQuery(faces []FaceEmbedding, idx int, eps float64) []int {
	var out []int
	for j, f := range faces {
		if j != idx && cosineDist(faces[idx].Embedding, f.Embedding) <= eps {
			out = append(out, j)
		}
	}
	return out
}

func cosineDist(a, b []float32) float64 {
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	d := 1.0 - dot
	if d < 0 {
		d = 0
	}
	return math.Abs(d)
}
