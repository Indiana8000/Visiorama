package repositories

import (
	"database/sql"
	"encoding/binary"
	"math"
)

type Person struct {
	ID          int64
	Name        string
	CoverFaceID *int64
	CoverCrop   *string
	CreatedAt   string
	FaceCount   int
	MediaCount  int
}

type FaceCluster struct {
	ClusterID int
	Faces     []ClusterFace
}

type ClusterFace struct {
	FaceID   int64
	MediaID  int64
	CropPath string
	BBoxJSON string
}

type PersonsRepo struct {
	db *sql.DB
}

func NewPersonsRepo(db *sql.DB) *PersonsRepo {
	return &PersonsRepo{db: db}
}

// AllFaceEmbeddings loads every face row with its embedding for clustering.
// Returns only faces that have no confirmed person assignment yet.
func (r *PersonsRepo) UnassignedFaceEmbeddings() ([]FaceWithEmbedding, error) {
	rows, err := r.db.Query(`
		SELECT f.id, f.embedding
		FROM ai_faces f
		LEFT JOIN ai_face_assignments a ON a.face_id = f.id
		WHERE a.face_id IS NULL OR a.confirmed = 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FaceWithEmbedding
	for rows.Next() {
		var id int64
		var blob []byte
		if err := rows.Scan(&id, &blob); err != nil {
			return nil, err
		}
		out = append(out, FaceWithEmbedding{ID: id, Embedding: blobToF32(blob)})
	}
	return out, rows.Err()
}

// AllFaceEmbeddings returns ALL face embeddings (for full re-cluster).
func (r *PersonsRepo) AllFaceEmbeddings() ([]FaceWithEmbedding, error) {
	rows, err := r.db.Query(`SELECT id, embedding FROM ai_faces`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FaceWithEmbedding
	for rows.Next() {
		var id int64
		var blob []byte
		if err := rows.Scan(&id, &blob); err != nil {
			return nil, err
		}
		out = append(out, FaceWithEmbedding{ID: id, Embedding: blobToF32(blob)})
	}
	return out, rows.Err()
}

type FaceWithEmbedding struct {
	ID        int64
	Embedding []float32
}

// SaveClusterAssignments writes DBSCAN results as unconfirmed assignments.
// Existing unconfirmed assignments for the given face IDs are replaced.
// Noise faces (clusterID == -1) are left unassigned.
// clusterIDOffset makes cluster IDs unique across runs; callers pass max(existing person IDs).
func (r *PersonsRepo) SaveClusterAssignments(assignments map[int64]int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	// Remove all unconfirmed assignments and their orphaned unnamed persons.
	if _, err := tx.Exec(`
		DELETE FROM ai_persons
		WHERE name = ''
		  AND id NOT IN (SELECT person_id FROM ai_face_assignments WHERE confirmed = 1)`); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`DELETE FROM ai_face_assignments WHERE confirmed = 0`); err != nil {
		_ = tx.Rollback()
		return err
	}

	// Group by cluster, upsert temp person rows for new clusters.
	clusters := map[int][]int64{}
	for faceID, cID := range assignments {
		if cID < 0 {
			continue
		}
		clusters[cID] = append(clusters[cID], faceID)
	}

	insStmt, err := tx.Prepare(`
		INSERT INTO ai_face_assignments (face_id, person_id, confirmed)
		VALUES (?, ?, 0)
		ON CONFLICT(face_id) DO UPDATE SET person_id = excluded.person_id, confirmed = 0`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer insStmt.Close()

	for _, faceIDs := range clusters {
		// Create a new unnamed person for this cluster.
		res, err := tx.Exec(
			`INSERT INTO ai_persons (name, created_at) VALUES ('', datetime('now'))`)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		personID, _ := res.LastInsertId()
		// Set cover to first face.
		if _, err := tx.Exec(`UPDATE ai_persons SET cover_face_id = ? WHERE id = ?`,
			faceIDs[0], personID); err != nil {
			_ = tx.Rollback()
			return err
		}
		for _, faceID := range faceIDs {
			if _, err := insStmt.Exec(faceID, personID); err != nil {
				_ = tx.Rollback()
				return err
			}
		}
	}
	return tx.Commit()
}

// ListClusters returns clusters that are not yet named (person.name == '').
func (r *PersonsRepo) ListClusters() ([]FaceCluster, error) {
	rows, err := r.db.Query(`
		SELECT p.id, f.id, f.media_id, f.crop_path, f.bbox_json
		FROM ai_persons p
		JOIN ai_face_assignments a ON a.person_id = p.id
		JOIN ai_faces f ON f.id = a.face_id
		WHERE p.name = ''
		ORDER BY p.id, f.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	clusterMap := map[int][]ClusterFace{}
	clusterOrder := []int{}
	seen := map[int]bool{}
	for rows.Next() {
		var personID int
		var cf ClusterFace
		if err := rows.Scan(&personID, &cf.FaceID, &cf.MediaID, &cf.CropPath, &cf.BBoxJSON); err != nil {
			return nil, err
		}
		if !seen[personID] {
			seen[personID] = true
			clusterOrder = append(clusterOrder, personID)
		}
		clusterMap[personID] = append(clusterMap[personID], cf)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]FaceCluster, 0, len(clusterOrder))
	for _, id := range clusterOrder {
		out = append(out, FaceCluster{ClusterID: id, Faces: clusterMap[id]})
	}
	return out, nil
}

// NameCluster assigns a name to a cluster (person_id) and confirms all its assignments.
func (r *PersonsRepo) NameCluster(personID int64, name string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE ai_persons SET name = ? WHERE id = ?`, name, personID); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`UPDATE ai_face_assignments SET confirmed = 1 WHERE person_id = ?`, personID); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// RemoveFaceFromCluster deletes an unconfirmed face assignment.
func (r *PersonsRepo) RemoveFaceFromCluster(faceID int64) error {
	_, err := r.db.Exec(`
		DELETE FROM ai_face_assignments WHERE face_id = ? AND confirmed = 0`, faceID)
	return err
}

// ListPersons returns all named persons with face/media counts and cover crop.
func (r *PersonsRepo) ListPersons() ([]Person, error) {
	rows, err := r.db.Query(`
		SELECT p.id, p.name, p.cover_face_id, f.crop_path, p.created_at,
		       COUNT(DISTINCT a.face_id), COUNT(DISTINCT fa.media_id)
		FROM ai_persons p
		LEFT JOIN ai_faces f ON f.id = p.cover_face_id
		LEFT JOIN ai_face_assignments a ON a.person_id = p.id
		LEFT JOIN ai_faces fa ON fa.id = a.face_id
		WHERE p.name != ''
		GROUP BY p.id
		ORDER BY p.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Person
	for rows.Next() {
		var p Person
		var crop sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &p.CoverFaceID, &crop,
			&p.CreatedAt, &p.FaceCount, &p.MediaCount); err != nil {
			return nil, err
		}
		if crop.Valid {
			p.CoverCrop = &crop.String
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetPersonMedia returns paged media IDs for a person.
func (r *PersonsRepo) GetPersonMedia(personID int64, page, pageSize int) ([]int64, int, error) {
	offset := (page - 1) * pageSize
	var total int
	if err := r.db.QueryRow(`
		SELECT COUNT(DISTINCT f.media_id)
		FROM ai_face_assignments a
		JOIN ai_faces f ON f.id = a.face_id
		WHERE a.person_id = ?`, personID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(`
		SELECT DISTINCT f.media_id
		FROM ai_face_assignments a
		JOIN ai_faces f ON f.id = a.face_id
		WHERE a.person_id = ?
		LIMIT ? OFFSET ?`, personID, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, 0, err
		}
		ids = append(ids, id)
	}
	return ids, total, rows.Err()
}

// MergePersons moves all face assignments from src to dst, then deletes src.
func (r *PersonsRepo) MergePersons(dstID, srcID int64) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`
		UPDATE ai_face_assignments SET person_id = ? WHERE person_id = ?`, dstID, srcID); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`DELETE FROM ai_persons WHERE id = ?`, srcID); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// DeletePerson removes a person and all their face assignments (faces stay).
func (r *PersonsRepo) DeletePerson(personID int64) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM ai_face_assignments WHERE person_id = ?`, personID); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`DELETE FROM ai_persons WHERE id = ?`, personID); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// UnclusteredCount returns count of faces that have no assignment at all.
func (r *PersonsRepo) UnclusteredCount() (int, error) {
	var n int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM ai_faces f
		LEFT JOIN ai_face_assignments a ON a.face_id = f.id
		WHERE a.face_id IS NULL`).Scan(&n)
	return n, err
}

// PendingClusterCount returns number of unnamed clusters.
func (r *PersonsRepo) PendingClusterCount() (int, error) {
	var n int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM ai_persons WHERE name = ''`).Scan(&n)
	return n, err
}

func blobToF32(b []byte) []float32 {
	if len(b)%4 != 0 {
		return nil
	}
	out := make([]float32, len(b)/4)
	for i := range out {
		bits := binary.LittleEndian.Uint32(b[i*4:])
		out[i] = math.Float32frombits(bits)
	}
	return out
}
