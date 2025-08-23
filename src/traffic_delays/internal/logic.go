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
			LineName:  record.ValueByKey("line_name").(string),
			ModeName:  record.ValueByKey("mode_name").(string),
			Timestamp: record.Time(),
		}

		// Populate fields based on alert type
		if alertType == "CriticalDelay" {
			alert.StatusDescription = record.ValueByKey("status_severity_description").(string)
			if reason, ok := record.ValueByKey("reason").(string); ok {
				alert.Reason = reason
			}
		} else if alertType == "SuddenServiceWorsening" {
			alert.SeverityDrop = record.ValueByKey("_value").(float64)
			// For sudden worsening, statusDescription and reason might not be directly relevant
			// or could be derived in the consumer if needed.
		}

		alerts = append(alerts, alert)
	}

	if result.Err() != nil {
		return nil, fmt.Errorf("error during Flux query result iteration (%s): %w", alertType, result.Err())
	}

	return alerts, nil
}
