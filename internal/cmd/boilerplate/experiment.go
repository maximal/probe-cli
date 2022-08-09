package main

//
// Code to generate a new experiment.
//

import (
	_ "embed"
	"path/filepath"
	"text/template"

	"github.com/AlecAivazis/survey/v2"
	"github.com/ooni/probe-cli/v3/internal/runtimex"
	"github.com/spf13/cobra"
)

// Implements interactively generating a new experiment.
type NewExperimentCommand struct{}

// Information about the experiment to create.
type ExperimentInfo struct {
	// Experiment name
	Name string

	// Experiment version
	Version string

	// Experiment spec URL
	SpecURL string

	// Experiment input policy
	InputPolicy string

	// Whether this experiment is interruptible.
	Interruptible bool
}

// Called by the CLI parser
func (c *NewExperimentCommand) Run(*cobra.Command, []string) {
	printf("\n")
	printf("Welcome! This command will help you to automatically generate code\n")
	printf("implementing a new OONI network experiment!\n")
	print("\n")

	info := getExperimentInfo()

	printf("\n")
	printf("Thank you! Now I'm going to generate boilerplate code for the new experiment!\n")
	printf("\n")

	makeExperimentDirectory(info)
	generateDocGo(info)
	generateMeasurerGo(info)
	generateModelsGo(info)
	generateTasksGo(info)
	generateMainTaskGo(info)
	generateRegistryEntryGo(info)
	if info.InputPolicy != "InputNone" {
		generateInputParserGo(info)
	}

	pkg := filepath.Join("internal", "experiment", info.Name, "/...")
	gofmt(pkg)

	printf("\n")
	printf("🏁 All done! Now you can try:\n")
	printf("\n")
	printf("* `go build -v ./internal/cmd/miniooni` to build `miniooni`;\n")
	printf("\n")
	printf("* `./miniooni -n %s` to test your new experiment;\n", info.Name)
	printf("\n")
	printf("* `go run ./internal/cmd/boilerplate new-task` to add tasks\n")
	printf("  to your new experiment (e.g., a TLS handshake).\n")
	printf("\n")
}

// Obtains the experiment info
func getExperimentInfo() *ExperimentInfo {
	return &ExperimentInfo{
		Name:          getExperimentName(),
		Version:       getExperimentVersion(),
		SpecURL:       getExperimentSpecURL(),
		InputPolicy:   getExperimentInputPolicy(),
		Interruptible: getExperimentInterruptible(),
	}
}

// Obtains the experiment name
func getExperimentName() string {
	printf("Each OONI experiment has a name, which should match [a-z]+. The experiment\n")
	printf("name determines the Go package name and the name with which you're calling the\n")
	printf("experiment by name from the command line.\n")
	print("\n")
	prompt := &survey.Input{
		Message: "Experiment's name:",
	}
	var experiment string
	err := survey.AskOne(prompt, &experiment)
	runtimex.PanicOnError(err, "survey.AskOne failed")
	return experiment
}

// Obtains the experiment version
func getExperimentVersion() string {
	print("\n")
	printf("Each OONI experiment has a <major>.<minor>.<patch> version number.\n")
	print("\n")
	prompt := &survey.Input{
		Message: "Experiment's version:",
	}
	var version string
	err := survey.AskOne(prompt, &version)
	runtimex.PanicOnError(err, "survey.AskOne failed")
	return version
}

// Obtains the experiment spec URL
func getExperimentSpecURL() string {
	print("\n")
	printf("Any OONI experiment should be associated with a public specification URL\n")
	printf("describing the experiment design and implementation.\n")
	print("\n")
	printf("Typically, specs live at https://github.com/ooni/spec/tree/master/nettests.\n")
	print("\n")
	prompt := &survey.Input{
		Message: "Experiment's spec URL:",
	}
	var specURL string
	err := survey.AskOne(prompt, &specURL)
	runtimex.PanicOnError(err, "survey.AskOne failed")
	return specURL
}

// Obtains the experiment input policy.
func getExperimentInputPolicy() string {
	print("\n")
	printf("Each OONI experiment has a specific policy regarding input, which is one of:\n")
	print("\n")
	printf("* InputOrQueryBackend: the user can specify input using --input or --input-file, but, if\n")
	printf("  input is missing, the experiment will query the OONI backend to obtain input.\n")
	print("\n")
	printf("* InputOrStaticDefault: the user can specify input using --input or --input-file, but, if\n")
	printf("  input is missing, the experiment will a static list bundled with the probe.\n")
	print("\n")
	printf("* InputStrictlyRequired: the user can specify input using --input or --input-file, and, if\n")
	printf("  input is missing, the experiment will emit an error and refuse to run.\n")
	print("\n")
	printf("* InputStrictlyRequired: the user cannot specify any input.\n")
	print("\n")
	var inputPolicy string
	prompt := &survey.Select{
		Message: "Choose an experiment input policy:",
		Options: []string{
			"InputOrQueryBackend",
			"InputOrStaticDefault",
			"InputStrictlyRequired",
			"InputNone",
		},
	}
	err := survey.AskOne(prompt, &inputPolicy)
	runtimex.PanicOnError(err, "survey.AskOne failed")
	return inputPolicy
}

// Returns whether we can interrupt experiments midway.
func getExperimentInterruptible() bool {
	print("\n")
	printf("Most OONI experiments runs short measurements. For such experiments, we do not\n")
	printf("want the engine to be able to interrupt a measurement. Rather, we have well defined\n")
	printf("interruption points between measuring an input and measuring the next one.\n")
	print("\n")
	printf("Though, network performance experiments and, generally, all the experiments whose\n")
	printf("measurements could last for dozens of seconds, are interruptible.\n")
	print("\n")
	var interruptible bool
	prompt := &survey.Confirm{
		Message: "Should the engine be able to abruptly interrupt a measurement?",
	}
	err := survey.AskOne(prompt, &interruptible)
	runtimex.PanicOnError(err, "survey.AskOne failed")
	return interruptible
}

// Creates a directory for the new experiment.
func makeExperimentDirectory(info *ExperimentInfo) {
	fulldir := filepath.Join("internal", "experiment", info.Name)
	mkdirP(fulldir)
}

//go:embed "experiment/doc.go.txt"
var experimentDocGoTemplate string

// Generates the doc.go file
func generateDocGo(info *ExperimentInfo) {
	fullpath := filepath.Join("internal", "experiment", info.Name, "doc.go")
	tmpl := template.Must(template.New("doc.go").Parse(experimentDocGoTemplate))
	writeTemplate(fullpath, tmpl, info)
}

//go:embed "experiment/measurer.go.txt"
var experimentMeasurerGoTemplate string

// Generates the measurer.go file
func generateMeasurerGo(info *ExperimentInfo) {
	fullpath := filepath.Join("internal", "experiment", info.Name, "measurer.go")
	tmpl := template.Must(template.New("measurer.go").Parse(experimentMeasurerGoTemplate))
	writeTemplate(fullpath, tmpl, info)
}

//go:embed "experiment/config.go.txt"
var experimentConfigGoTemplate string

//go:embed "experiment/summary.go.txt"
var experimentSummaryGoTemplate string

//go:embed "experiment/testkeys.go.txt"
var experimentTestkeysGoTemplate string

// Generates the model.go file
func generateModelsGo(info *ExperimentInfo) {
	{
		fullpath := filepath.Join("internal", "experiment", info.Name, "config.go")
		tmpl := template.Must(template.New("config.go").Parse(experimentConfigGoTemplate))
		writeTemplate(fullpath, tmpl, info)
	}
	{
		fullpath := filepath.Join("internal", "experiment", info.Name, "summary.go")
		tmpl := template.Must(template.New("model.go").Parse(experimentSummaryGoTemplate))
		writeTemplate(fullpath, tmpl, info)
	}
	{
		fullpath := filepath.Join("internal", "experiment", info.Name, "testkeys.go")
		tmpl := template.Must(template.New("model.go").Parse(experimentTestkeysGoTemplate))
		writeTemplate(fullpath, tmpl, info)
	}
}

//go:embed "experiment/tasks.go.txt"
var experimentTasksGoTemplate string

// Generates the tasks.go file
func generateTasksGo(info *ExperimentInfo) {
	fullpath := filepath.Join("internal", "experiment", info.Name, "tasks.go")
	tmpl := template.Must(template.New("tasks.go").Parse(experimentTasksGoTemplate))
	writeTemplate(fullpath, tmpl, info)
}

//go:embed "experiment/maintask.go.txt"
var experimentMainTaskGoTemplate string

// Generates the maintask.go file
func generateMainTaskGo(info *ExperimentInfo) {
	fullpath := filepath.Join("internal", "experiment", info.Name, "maintask.go")
	tmpl := template.Must(template.New("maintask.go").Parse(experimentMainTaskGoTemplate))
	writeTemplate(fullpath, tmpl, info)
}

//go:embed "experiment/registry.go.txt"
var experimentRegistryEntryGoTemplate string

// Generates the experiment's entry inside ./internal/registry
func generateRegistryEntryGo(info *ExperimentInfo) {
	fullpath := filepath.Join("internal", "registry", info.Name+".go")
	tmpl := template.Must(template.New("registryentry.go").Parse(experimentRegistryEntryGoTemplate))
	writeTemplate(fullpath, tmpl, info)
}

//go:embed "experiment/inputparser.go.txt"
var experimentInputParserGoTemplate string

// Generates the experiment's entry inside ./internal/registry
func generateInputParserGo(info *ExperimentInfo) {
	fullpath := filepath.Join("internal", "experiment", info.Name, "inputparser.go")
	tmpl := template.Must(template.New("inputparser.go").Parse(experimentInputParserGoTemplate))
	writeTemplate(fullpath, tmpl, info)
}
