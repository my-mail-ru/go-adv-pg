/*
Command adv-pg runs the code-first SQL query generator with ActiveRecord support.

See [github.com/my-mail-ru/go-adv-pg] for a detailed description of the
code generator input and output. Here's a command-line interface description only.

# Typical usage

After installing the adv-pg module for your project:

	go get -tool github.com/my-mail-ru/go-adv-pg/cmd/adv-pg

Add a //go:generate line in all the source files containing the database models:

	//go:generate go tool adv-pg

Then issue a go generate command:

	go generate ./...

That's all!

# Flags

These flags are intended for debugging the adv-pg itself or preparing a bug report to its author(s):

  - -in (default: $GOFILE environment variable). The input is the go source file. Required.
  - -no-format (default: formatting is enabled). Do not run the goimports on the output.
    Use when an incorrect code is generated to report a bug (goimports may fail on broken code).
  - -stdout (default: append _generated suffix to the source file name). Write the output to [os.Stdout].
*/
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	advpggen "github.com/my-mail-ru/go-adv-pg/internal/gen"
)

var (
	inFile            = flag.String("in", os.Getenv("GOFILE"), "input file (default: $GOFILE)")
	disableFormatting = flag.Bool("no-format", false, "disable output formatting")
	writeToStdout     = flag.Bool("stdout", false, "write output to stdout")
)

func main() {
	flag.Parse()

	if *inFile == "" {
		log.Fatal("adv-pg: -in/$GOFILE is required")
	}

	root, err := os.OpenRoot(filepath.Dir(*inFile))
	if err != nil {
		log.Fatal("adv-pg: ", err)
	}

	models, err := advpggen.Parse(root.FS(), filepath.Base(*inFile))
	if err != nil {
		log.Fatal(err)
	}

	var opts []advpggen.GenerateOptions

	if !*disableFormatting {
		opts = append(opts, advpggen.WithGoimports)
	}

	fw := advpggen.FileWriter(fileWriter{})
	if *writeToStdout {
		fw = advpggen.NewWriterWriter(os.Stdout)
	}

	if err = models.Generate(fw, opts...); err != nil {
		log.Fatal(err)
	}
}

type fileWriter struct{}

func (fileWriter) WriteFile(fname string, data []byte) error {
	err := os.WriteFile(fname, data, 0o644) //nolint:gosec
	if err != nil {
		return fmt.Errorf("adv-pg: %s: %w", fname, err)
	}

	log.Println("adv-pg: writing", fname)

	return nil
}
