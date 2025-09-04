package internal

import (
	"context"
	"fmt"
	"github.com/matteoavallone7/optimaLDN/src/common"
)

func ExecuteAndProcessQuery(ctx context.Context, fluxQuery string, alertType string) ([]common.TfLAlert, error) {
	var alerts []common.TfLAlert
	result, err := InfluxQueryAPI.Query(ctx, fluxQuery)
	if err != nil {
		return nil, fmt.Errorf("error executing Flux query (%s): %w", alertType, err)
	}

	for result.Next() {
		record := result.Record()
		alert := common.TfLAlert{
			Timestamp: record.Time(),
		}

		if v, ok := record.ValueByKey("line_name").(string); ok {
			alert.LineName = v
		}
		if v, ok := record.ValueByKey("mode_name").(string); ok {
			alert.ModeName = v
		}

		switch alertType {
		case "Critical Delay":
			if v, ok := record.ValueByKey("status_severity_description").(string); ok {
				alert.StatusDescription = v
			}
			if v, ok := record.ValueByKey("reason").(string); ok {
				alert.Reason = v
			}
		case "Sudden Service Worsening":
			if v, ok := record.ValueByKey("_value").(float64); ok {
				alert.SeverityDrop = v
			}
		}

		alerts = append(alerts, alert)
	}

	if result.Err() != nil {
		return nil, fmt.Errorf("error during Flux query result iteration (%s): %w", alertType, result.Err())
	}

	return alerts, nil
}
