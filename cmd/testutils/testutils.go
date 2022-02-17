// Utilities for _test.go files
package testutils

import (
	"fmt"
	"github.com/k0kubun/sqldef/adapter"
	"github.com/k0kubun/sqldef/schema"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type TestCase struct {
	Current string // default: empty schema
	Desired string // default: empty schema
	Output  string // default: use Desired as Output
}

func ReadTests(pattern string) (map[string]TestCase, error) {
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	ret := map[string]TestCase{}
	for _, file := range files {
		var tests map[string]*TestCase

		buf, err := ioutil.ReadFile(file)
		if err != nil {
			return nil, err
		}

		err = yaml.UnmarshalStrict(buf, &tests)
		if err != nil {
			return nil, err
		}

		for name, test := range tests {
			if test.Output == "" {
				test.Output = test.Desired
			}
			if _, ok := ret[name]; ok {
				log.Fatal(fmt.Sprintf("There are multiple test cases named '%s'", name))
			}
			ret[name] = *test
		}
	}

	return ret, nil
}

func RunTest(t *testing.T, db adapter.Database, test TestCase, mode schema.GeneratorMode) {
	// Prepare current
	if test.Current != "" {
		ddls, err := SplitDDLs(mode, test.Current)
		if err != nil {
			t.Fatal(err)
		}
		err = RunDDLs(db, ddls)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Test idempotency
	dumpDDLs, err := adapter.DumpDDLs(db)
	if err != nil {
		log.Fatal(err)
	}
	ddls, err := schema.GenerateIdempotentDDLs(mode, test.Current, dumpDDLs)
	if err != nil {
		t.Fatal(err)
	}
	if len(ddls) > 0 {
		t.Errorf("expected nothing is modifed, but got:\n```\n%s```", JoinDDLs(ddls))
	}

	// Main test
	dumpDDLs, err = adapter.DumpDDLs(db)
	if err != nil {
		log.Fatal(err)
	}
	ddls, err = schema.GenerateIdempotentDDLs(mode, test.Desired, dumpDDLs)
	if err != nil {
		t.Fatal(err)
	}
	expected := test.Output
	actual := JoinDDLs(ddls)
	if expected != actual {
		t.Errorf("\nexpected:\n```\n%s```\n\nactual:\n```\n%s```", expected, actual)
	}
	err = RunDDLs(db, ddls)
	if err != nil {
		t.Fatal(err)
	}

	// Test idempotency
	dumpDDLs, err = adapter.DumpDDLs(db)
	if err != nil {
		log.Fatal(err)
	}
	ddls, err = schema.GenerateIdempotentDDLs(mode, test.Desired, dumpDDLs)
	if err != nil {
		t.Fatal(err)
	}
	if len(ddls) > 0 {
		t.Errorf("expected nothing is modifed, but got:\n```\n%s```", JoinDDLs(ddls))
	}
}

func SplitDDLs(mode schema.GeneratorMode, str string) ([]string, error) {
	statements, err := schema.ParseDDLs(mode, str)
	if err != nil {
		return nil, err
	}

	var ddls []string
	for _, statement := range statements {
		ddls = append(ddls, statement.Statement())
	}
	return ddls, nil
}

func RunDDLs(db adapter.Database, ddls []string) error {
	transaction, err := db.DB().Begin()
	if err != nil {
		return err
	}
	for _, ddl := range ddls {
		if _, err := transaction.Exec(ddl); err != nil {
			rollbackErr := transaction.Rollback()
			if rollbackErr != nil {
				return rollbackErr
			}
			return err
		}
	}
	return transaction.Commit()
}

func JoinDDLs(ddls []string) string {
	var builder strings.Builder
	for _, ddl := range ddls {
		builder.WriteString(ddl)
		builder.WriteString(";\n")
	}
	return builder.String()
}

func MustExecute(command string, args ...string) string {
	out, err := execute(command, args...)
	if err != nil {
		log.Printf("failed to execute '%s %s': `%s`", command, strings.Join(args, " "), out)
		log.Fatal(err)
	}
	return out
}

func execute(command string, args ...string) (string, error) {
	cmd := exec.Command(command, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}