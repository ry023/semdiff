package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ry023/semdiff/internal/groups"
	"github.com/ry023/semdiff/internal/questions"
)

func runQuestions(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("questions requires wait or answer")
	}
	switch args[0] {
	case "wait":
		fs := flag.NewFlagSet("questions wait", flag.ContinueOnError)
		jsonOut := fs.Bool("json", false, "JSON output")
		positional, err := parseInterspersed(fs, args[1:])
		if err != nil {
			return err
		}
		if len(positional) != 1 {
			return errors.New("questions wait requires <groups-file>")
		}
		store, err := questionStore(positional[0])
		if err != nil {
			return err
		}
		question, err := store.Wait(ctx)
		if err != nil {
			return err
		}
		if *jsonOut {
			return printJSON(question)
		}
		fmt.Printf("%s  %s\n", question.ID, question.Question)
		return nil
	case "answer":
		fs := flag.NewFlagSet("questions answer", flag.ContinueOnError)
		stdin := fs.Bool("stdin", false, "read answer from stdin")
		jsonOut := fs.Bool("json", false, "JSON output")
		positional, err := parseInterspersed(fs, args[1:])
		if err != nil {
			return err
		}
		if len(positional) != 2 || !*stdin {
			return errors.New("questions answer requires <groups-file> <question-id> --stdin")
		}
		body, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(body)) == "" {
			return errors.New("answer from stdin is empty")
		}
		store, err := questionStore(positional[0])
		if err != nil {
			return err
		}
		question, err := store.Answer(positional[1], string(body))
		if err != nil {
			return err
		}
		if *jsonOut {
			return printJSON(question)
		}
		fmt.Printf("answered %s\n", question.ID)
		return nil
	default:
		return fmt.Errorf("unknown questions command %q", args[0])
	}
}

func questionStore(groupsPath string) (questions.Store, error) {
	file, err := groups.Load(groupsPath)
	if err != nil {
		return questions.Store{}, err
	}
	return questions.Store{
		Path: questions.DefaultPath(groupsPath, file.BaseSHA, file.HeadSHA), BaseSHA: file.BaseSHA, HeadSHA: file.HeadSHA,
	}, nil
}
