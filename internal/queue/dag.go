package queue

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type QueryRower interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func CheckCyclicDependency(ctx context.Context, db QueryRower, jobID, dependsOnJobID string) error {
	if jobID == dependsOnJobID {
		return fmt.Errorf("job cannot depend on itself")
	}

	var cycleExists bool
	err := db.QueryRow(ctx, `
		WITH RECURSIVE dep_chain AS (
			SELECT depends_on_job_id AS ancestor_id
			FROM job_dependencies
			WHERE job_id = $2

			UNION ALL

			SELECT jd.depends_on_job_id
			FROM job_dependencies jd
			INNER JOIN dep_chain dc ON jd.job_id = dc.ancestor_id
		)
		SELECT EXISTS (
			SELECT 1 FROM dep_chain WHERE ancestor_id = $1
		)
	`, jobID, dependsOnJobID).Scan(&cycleExists)
	if err != nil {
		return fmt.Errorf("failed to verify dependency graph: %w", err)
	}

	if cycleExists {
		return fmt.Errorf("circular dependency detected: job %s would create a cycle through ancestor %s", jobID, dependsOnJobID)
	}

	return nil
}
