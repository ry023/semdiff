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
	case "session":
		if len(args) < 2 || args[1] != "start" {
			return errors.New("questions session requires start")
		}
		fs := flag.NewFlagSet("questions session start", flag.ContinueOnError)
		jsonOut := fs.Bool("json", false, "JSON output")
		draftPath := fs.String("draft", defaultGroupingDraftPath, "draft path used to locate the default groups file")
		positional, err := parseInterspersed(fs, args[2:])
		if err != nil {
			return err
		}
		if len(positional) > 1 {
			return errors.New("questions session start accepts at most one <groups-file>")
		}
		groupsPath, err := resolveQuestionGroupsPath(positional, *draftPath)
		if err != nil {
			return err
		}
		store, err := questionStore(groupsPath)
		if err != nil {
			return err
		}
		session, err := store.Sessions().Start()
		if err != nil {
			return err
		}
		if *jsonOut {
			return printJSON(session)
		}
		fmt.Printf("started %s\n", session.ID)
		return nil
	case "wait":
		fs := flag.NewFlagSet("questions wait", flag.ContinueOnError)
		jsonOut := fs.Bool("json", false, "JSON output")
		sessionID := fs.String("session", "", "answer session ID")
		draftPath := fs.String("draft", defaultGroupingDraftPath, "draft path used to locate the default groups file")
		positional, err := parseInterspersed(fs, args[1:])
		if err != nil {
			return err
		}
		if len(positional) > 1 {
			return errors.New("questions wait accepts at most one <groups-file>")
		}
		groupsPath, err := resolveQuestionGroupsPath(positional, *draftPath)
		if err != nil {
			return err
		}
		store, err := questionStore(groupsPath)
		if err != nil {
			return err
		}
		if *sessionID != "" {
			event, err := store.WaitSession(ctx, *sessionID)
			if err != nil {
				return err
			}
			if *jsonOut {
				return printJSON(event)
			}
			if event.Event == "stopped" {
				fmt.Printf("stopped %s\n", event.SessionID)
			} else {
				fmt.Printf("%s  %s\n", event.Question.ID, event.Question.Question)
			}
			return nil
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
		draftPath := fs.String("draft", defaultGroupingDraftPath, "draft path used to locate the default groups file")
		positional, err := parseInterspersed(fs, args[1:])
		if err != nil {
			return err
		}
		if (len(positional) != 1 && len(positional) != 2) || !*stdin {
			return errors.New("questions answer requires [<groups-file>] <question-id> --stdin")
		}
		groupsPath, questionID := "", ""
		if len(positional) == 2 {
			groupsPath, questionID = positional[0], positional[1]
		} else {
			groupsPath, err = defaultGroupsPath(*draftPath)
			if err != nil {
				return fmt.Errorf("locate default groups file from draft: %w", err)
			}
			questionID = positional[0]
		}
		body, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(body)) == "" {
			return errors.New("answer from stdin is empty")
		}
		store, err := questionStore(groupsPath)
		if err != nil {
			return err
		}
		thread, err := store.Answer(questionID, string(body))
		if err != nil {
			return err
		}
		if *jsonOut {
			return printJSON(thread)
		}
		fmt.Printf("answered %s\n", questionID)
		return nil
	default:
		return fmt.Errorf("unknown questions command %q", args[0])
	}
}

func resolveQuestionGroupsPath(positional []string, draftPath string) (string, error) {
	if len(positional) == 1 {
		return positional[0], nil
	}
	groupsPath, err := defaultGroupsPath(draftPath)
	if err != nil {
		return "", fmt.Errorf("locate default groups file from draft: %w", err)
	}
	return groupsPath, nil
}

func questionStore(groupsPath string) (questions.Store, error) {
	file, err := groups.Load(groupsPath)
	if err != nil {
		return questions.Store{}, err
	}
	return questions.Store{
		Path: questions.DefaultPath(groupsPath, file.BaseSHA, file.HeadSHA), SessionPath: questions.DefaultSessionPath(groupsPath, file.BaseSHA, file.HeadSHA), BaseSHA: file.BaseSHA, HeadSHA: file.HeadSHA,
	}, nil
}
