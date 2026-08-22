package framework

import (
	"errors"
	"fmt"
	"os"
	"text/tabwriter"

	go_console "github.com/DrSmithFr/go-console"
	"github.com/DrSmithFr/go-console/question"
	"github.com/DrSmithFr/go-console/question/answers"
	"github.com/manifoldco/promptui"
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

func (e *Execution) AskWithChoices(prompt string, choices []string) (int, error) {
	sel := promptui.Select{
		Label: prompt,
		Items: choices,
	}

	result, _, err := sel.Run()
	if err != nil {
		return 0, err
	}

	return result, nil
}

func (e *Execution) AskPassword(prompt string) (string, error) {
	password := e.Question.Ask(question.NewQuestion(prompt).SetHidden(true))
	if password == "" {
		return "", errors.New("password not provided")
	}

	return password, nil
}

func (e *Execution) Confirm(prompt string) bool {
	answer := e.Question.Ask(
		question.
			NewComfirmation(prompt).
			SetDefaultAnswer(answers.Yes).
			SetMaxAttempts(2), //nolint:mnd // no mnd
	)

	return answer == answers.Yes
}
