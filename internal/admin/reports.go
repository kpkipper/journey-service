package admin

import (
	"context"
	"encoding/json"
	"io"

	"github.com/kpkipper/journey-service/internal/services"
)

// PrintOpenReports writes every open report as one JSON line.
func PrintOpenReports(ctx context.Context, w io.Writer, s *services.ReportService) error {
	reports, err := s.ListOpen(ctx)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	for i := range reports {
		if err := enc.Encode(&reports[i]); err != nil {
			return err
		}
	}
	return nil
}

// ResolveReport marks a report resolved (services.ErrReportNotFound if missing).
func ResolveReport(ctx context.Context, s *services.ReportService, id string) error {
	return s.Resolve(ctx, id)
}

// DeleteMoment removes a moment and its stored files, with no permission check.
func DeleteMoment(ctx context.Context, s *services.MomentService, id string) error {
	return s.AdminDelete(ctx, id)
}
