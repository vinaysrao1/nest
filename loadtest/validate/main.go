// Package main implements post-run validation for Nest integration tests.
// It queries PostgreSQL and the webhook receiver, compares counts, and prints
// a summary table. Exits 0 on pass, 1 on fail.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
)

// cliFlags holds all parsed command-line arguments.
type cliFlags struct {
	receiverURL       string
	databaseURL       string
	generatorSent     int
	generatorBlocks   int
	generatorReviews  int
}

// dbCounts collects the counts queried from PostgreSQL.
type dbCounts struct {
	totalItems       int64
	verdictBreakdown map[string]int64
	actionExecs      int64
}

// receiverStats mirrors the JSON returned by the receiver's /stats endpoint.
type receiverStats struct {
	Total    int64            `json:"total"`
	ByAction map[string]int64 `json:"by_action"`
	Errors   int64            `json:"errors"`
}

// validationResult holds the final comparison outcome.
type validationResult struct {
	dbItems         int64
	dbBlocks        int64
	dbReviews       int64
	dbActionExecs   int64
	webhookTotal    int64
	verdicts        map[string]int64
	itemsMissed     int64
	webhookMissed   int64
	webhookMissPct  float64
	pass            bool
}

func parseFlags() (cliFlags, error) {
	var f cliFlags
	flag.StringVar(&f.receiverURL, "receiver-url", "http://localhost:9090", "base URL of the webhook receiver")
	flag.StringVar(&f.databaseURL, "database-url",
		"postgres://nest:nest_test_pass@localhost:5433/nest_test?sslmode=disable",
		"PostgreSQL connection URL")
	flag.IntVar(&f.generatorSent, "generator-sent", 0, "number of items the generator sent (required)")
	flag.IntVar(&f.generatorBlocks, "generator-blocks", 0, "number of block verdicts the generator expected")
	flag.IntVar(&f.generatorReviews, "generator-reviews", 0, "number of review verdicts the generator expected")
	flag.Parse()

	if f.generatorSent <= 0 {
		return f, fmt.Errorf("-generator-sent is required and must be > 0")
	}
	return f, nil
}

// queryDB runs all three SQL queries against PostgreSQL and returns the counts.
// It uses rule_executions for verdict breakdown because the items table has no
// verdict column — verdicts are written by the rule evaluation engine.
func queryDB(ctx context.Context, connStr string) (dbCounts, error) {
	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return dbCounts{}, fmt.Errorf("connect to postgres: %w", err)
	}
	defer conn.Close(ctx)

	var counts dbCounts

	// Total distinct items submitted (deduplicated by (org_id, id, item_type_id, submission_id)).
	if err := conn.QueryRow(ctx, "SELECT COUNT(*) FROM items").Scan(&counts.totalItems); err != nil {
		return counts, fmt.Errorf("query items count: %w", err)
	}

	// Verdict breakdown: for each item pick the most-recent rule execution verdict.
	// We aggregate over rule_executions to find what the engine decided per item.
	const verdictSQL = `
		SELECT verdict, COUNT(*) AS cnt
		FROM (
			SELECT DISTINCT ON (org_id, item_id)
				org_id, item_id, verdict
			FROM rule_executions
			WHERE verdict IS NOT NULL
			ORDER BY org_id, item_id,
				CASE verdict
					WHEN 'block' THEN 3
					WHEN 'review' THEN 2
					WHEN 'approve' THEN 1
					ELSE 0
				END DESC
		) latest
		GROUP BY verdict`

	rows, err := conn.Query(ctx, verdictSQL)
	if err != nil {
		return counts, fmt.Errorf("query verdict breakdown: %w", err)
	}
	defer rows.Close()

	counts.verdictBreakdown = make(map[string]int64)
	for rows.Next() {
		var verdict string
		var cnt int64
		if scanErr := rows.Scan(&verdict, &cnt); scanErr != nil {
			return counts, fmt.Errorf("scan verdict row: %w", scanErr)
		}
		counts.verdictBreakdown[verdict] = cnt
	}
	if err := rows.Err(); err != nil {
		return counts, fmt.Errorf("iterate verdict rows: %w", err)
	}

	// Successful webhook action executions only (exclude MRT enqueue actions).
	const actionSQL = `
		SELECT COUNT(*)
		FROM action_executions ae
		JOIN actions a ON a.id = ae.action_id AND a.org_id = ae.org_id
		WHERE ae.success = true
		  AND a.action_type = 'WEBHOOK'`
	if err := conn.QueryRow(ctx, actionSQL).Scan(&counts.actionExecs); err != nil {
		return counts, fmt.Errorf("query action executions: %w", err)
	}

	return counts, nil
}

// fetchReceiverStats calls GET /stats on the webhook receiver and parses the response.
func fetchReceiverStats(receiverURL string) (receiverStats, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(receiverURL + "/stats")
	if err != nil {
		return receiverStats{}, fmt.Errorf("GET %s/stats: %w", receiverURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return receiverStats{}, fmt.Errorf("receiver /stats returned HTTP %d", resp.StatusCode)
	}

	var stats receiverStats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return stats, fmt.Errorf("decode receiver stats: %w", err)
	}
	return stats, nil
}

// compare builds the validation result from the raw counts and the CLI flags.
func compare(flags cliFlags, db dbCounts, recv receiverStats) validationResult {
	res := validationResult{
		dbItems:       db.totalItems,
		dbActionExecs: db.actionExecs,
		webhookTotal:  recv.Total,
		verdicts:      db.verdictBreakdown,
		itemsMissed:   int64(flags.generatorSent) - db.totalItems,
	}

	res.dbBlocks = db.verdictBreakdown["block"]
	res.dbReviews = db.verdictBreakdown["review"]

	res.webhookMissed = db.actionExecs - recv.Total
	if db.actionExecs > 0 {
		res.webhookMissPct = float64(res.webhookMissed) / float64(db.actionExecs) * 100.0
	}

	// Determine pass/fail:
	//   1. Items must all be present (no drops).
	//   2. Generator-specified block/review counts must match DB if non-zero.
	//   3. Webhook discrepancy must be < 1%.
	itemsMatch := res.itemsMissed == 0
	blocksMatch := flags.generatorBlocks == 0 || res.dbBlocks == int64(flags.generatorBlocks)
	reviewsMatch := flags.generatorReviews == 0 || res.dbReviews == int64(flags.generatorReviews)
	webhookOK := res.webhookMissPct < 1.0

	res.pass = itemsMatch && blocksMatch && reviewsMatch && webhookOK
	return res
}

// printSummary writes the formatted results table to stdout.
func printSummary(flags cliFlags, res validationResult) {
	fmt.Println("=== Nest Integration Test Results ===")
	fmt.Println()
	fmt.Println("Pipeline Counts:")
	fmt.Printf("  Generator sent:       %d\n", flags.generatorSent)

	var itemsLabel string
	switch {
	case res.itemsMissed > 0:
		itemsLabel = fmt.Sprintf("(%d dropped)", res.itemsMissed)
	case res.itemsMissed < 0:
		itemsLabel = fmt.Sprintf("(%d extra)", -res.itemsMissed)
	default:
		itemsLabel = "(0 dropped)"
	}
	fmt.Printf("  DB items:             %d  %s\n", res.dbItems, itemsLabel)
	fmt.Printf("  Action executions:     %d\n", res.dbActionExecs)

	missLabel := fmt.Sprintf("(%d missed, %.1f%%)", res.webhookMissed, res.webhookMissPct)
	if res.webhookMissed <= 0 {
		missLabel = "(0 missed, 0.0%)"
	}
	fmt.Printf("  Webhooks received:     %d  %s\n", res.webhookTotal, missLabel)

	fmt.Println()
	fmt.Println("Verdict Breakdown:")

	totalVerdicts := int64(0)
	for _, cnt := range res.verdicts {
		totalVerdicts += cnt
	}

	for _, v := range []string{"approve", "block", "review"} {
		cnt := res.verdicts[v]
		pct := 0.0
		if totalVerdicts > 0 {
			pct = float64(cnt) / float64(totalVerdicts) * 100.0
		}
		fmt.Printf("  %-8s %4d  (%4.1f%%)\n", v+":", cnt, pct)
	}

	// Print any verdicts not in the standard set.
	for v, cnt := range res.verdicts {
		if v == "approve" || v == "block" || v == "review" {
			continue
		}
		pct := 0.0
		if totalVerdicts > 0 {
			pct = float64(cnt) / float64(totalVerdicts) * 100.0
		}
		fmt.Printf("  %-8s %4d  (%4.1f%%)\n", v+":", cnt, pct)
	}

	fmt.Println()
	if res.pass {
		fmt.Println("Status: PASS")
	} else {
		fmt.Println("Status: FAIL")
		printFailureReasons(flags, res)
	}
}

// printFailureReasons lists each failing check to stderr for easy diagnosis.
func printFailureReasons(flags cliFlags, res validationResult) {
	if res.itemsMissed > 0 {
		fmt.Fprintf(os.Stderr, "  FAIL: %d items dropped (sent=%d, db=%d)\n",
			res.itemsMissed, flags.generatorSent, res.dbItems)
	} else if res.itemsMissed < 0 {
		fmt.Fprintf(os.Stderr, "  FAIL: %d extra items in DB (sent=%d, db=%d)\n",
			-res.itemsMissed, flags.generatorSent, res.dbItems)
	}
	if flags.generatorBlocks != 0 && res.dbBlocks != int64(flags.generatorBlocks) {
		fmt.Fprintf(os.Stderr, "  FAIL: block count mismatch (expected=%d, db=%d)\n",
			flags.generatorBlocks, res.dbBlocks)
	}
	if flags.generatorReviews != 0 && res.dbReviews != int64(flags.generatorReviews) {
		fmt.Fprintf(os.Stderr, "  FAIL: review count mismatch (expected=%d, db=%d)\n",
			flags.generatorReviews, res.dbReviews)
	}
	if res.webhookMissPct >= 1.0 {
		fmt.Fprintf(os.Stderr, "  FAIL: webhook miss rate %.2f%% >= 1%% (execs=%d, received=%d)\n",
			res.webhookMissPct, res.dbActionExecs, res.webhookTotal)
	}
}

func main() {
	flags, err := parseFlags()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		flag.Usage()
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Println("[validate] querying PostgreSQL...")
	db, err := queryDB(ctx, flags.databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[validate] db error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[validate] fetching receiver stats from %s...\n", flags.receiverURL)
	recv, err := fetchReceiverStats(flags.receiverURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[validate] receiver error: %v\n", err)
		os.Exit(1)
	}

	res := compare(flags, db, recv)
	printSummary(flags, res)

	if !res.pass {
		os.Exit(1)
	}
}
