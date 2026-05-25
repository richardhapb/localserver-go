package corrections

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/gin"
	"google.golang.org/api/iterator"
)

const (
	projectID    = "neospeller"
	collection   = "spellcheck_logs"
	defaultLimit = 100
	maxLimit     = 1000
)

// CreatedAt is stored as an RFC3339 string in Firestore (written by the
// Rust client). Keep it as a string here to avoid decode failures, and
// compare it lexicographically — RFC3339 sorts correctly as strings.
type SpellcheckLog struct {
	Original  string `firestore:"original" json:"original"`
	Corrected string `firestore:"corrected" json:"corrected"`
	CreatedAt string `firestore:"created_at" json:"created_at"`
}

var client *firestore.Client

func Init(ctx context.Context) error {
	c, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		return err
	}
	client = c
	return nil
}

func Close() {
	if client != nil {
		_ = client.Close()
	}
}

// List handles GET /corrections.
// Query params:
//   - limit: max records to return (default 100, max 1000)
//   - from:  RFC3339 timestamp, inclusive lower bound on created_at
//   - to:    RFC3339 timestamp, exclusive upper bound on created_at
//   - after: RFC3339 timestamp cursor; returns records strictly after this
//     created_at. Use the created_at of the last record from the previous
//     page to paginate forward.
//
// Results are ordered by created_at descending.
func List(c *gin.Context) {
	if client == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "firestore client not initialized"})
		return
	}

	limit := defaultLimit
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
			return
		}
		if n > maxLimit {
			n = maxLimit
		}
		limit = n
	}

	q := client.Collection(collection).OrderBy("created_at", firestore.Desc)

	if v := c.Query("from"); v != "" {
		if _, err := time.Parse(time.RFC3339, v); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid from timestamp"})
			return
		}
		q = q.Where("created_at", ">=", v)
	}
	if v := c.Query("to"); v != "" {
		if _, err := time.Parse(time.RFC3339, v); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid to timestamp"})
			return
		}
		q = q.Where("created_at", "<", v)
	}
	if v := c.Query("after"); v != "" {
		if _, err := time.Parse(time.RFC3339, v); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid after cursor"})
			return
		}
		q = q.StartAfter(v)
	}

	q = q.Limit(limit)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	it := q.Documents(ctx)
	defer it.Stop()

	out := make([]SpellcheckLog, 0, limit)
	for {
		doc, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("corrections: firestore query failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
			return
		}
		var l SpellcheckLog
		if err := doc.DataTo(&l); err != nil {
			log.Printf("corrections: decode failed for %s: %v", doc.Ref.ID, err)
			continue
		}
		out = append(out, l)
	}

	resp := gin.H{
		"count":      len(out),
		"limit":      limit,
		"items":      out,
	}
	if len(out) == limit {
		resp["next_after"] = out[len(out)-1].CreatedAt
	}
	c.JSON(http.StatusOK, resp)
}
