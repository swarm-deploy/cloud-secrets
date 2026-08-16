package framework

import (
	"errors"

	go_console "github.com/DrSmithFr/go-console"
	"github.com/DrSmithFr/go-console/question"
	"github.com/DrSmithFr/go-console/question/answers"
	"github.com/manifoldco/promptui"
)

type Execution struct {
	*go_console.Script

	Question *question.Helper
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
			SetMaxAttempts(2),
	)

	return answer == answers.Yes
}
