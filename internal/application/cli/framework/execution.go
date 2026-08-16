package framework

import (
	"fmt"
	"os"
	"text/tabwriter"

	go_console "github.com/DrSmithFr/go-console"
	"github.com/DrSmithFr/go-console/question"
)

type Execution struct {
	*go_console.Script

	Question *question.Helper
}

func (e *Execution) PrintTable(headers []string, rows [][]string) error {
	w := tabwriter.NewWriter(os.Stdout, 1, 1, 2, ' ', 0) //nolint:mnd // no mnd

	for _, header := range headers {
		_, err := fmt.Fprint(w, header+"\t")
		if err != nil {
			return fmt.Errorf("could not print header %q: %w", header, err)
		}
	}

	_, err := fmt.Fprintln(w, "")
	if err != nil {
		return fmt.Errorf("could not print delimiter: %w", err)
	}

	for _, row := range rows {
		for _, cell := range row {
			_, err = fmt.Fprint(w, cell+"\t")
			if err != nil {
				return fmt.Errorf("could not print cell %q: %w", cell, err)
			}
		}

		_, err = fmt.Fprintln(w, "")
		if err != nil {
			return fmt.Errorf("could not print row delimiter: %w", err)
		}
	}

	return w.Flush()
}
