package commands

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/CrowdSurge/banner"
	"github.com/spf13/cobra"
)

// config environment variable
const configENV = "QAZ_CONFIG"

// job var used as a central point for command data
var job = struct {
	cfgSource  string
	tplSource  string
	profile    string
	tplSources []string
	stacks     map[string]string
	all        bool
	version    bool
	request    string
	debug      bool
	funcEvent  string
	changeName string
	stackName  string
	rollback   bool
}{}

// Wait Group for handling goroutines
var wg sync.WaitGroup

// RootCmd command (calls all other commands)
var RootCmd = &cobra.Command{
	Use:   "qaz",
	Short: fmt.Sprintf("\n"),
	Run: func(cmd *cobra.Command, args []string) {

		if job.version {
			fmt.Printf("qaz - Version %s"+"\n", version)
			return
		}

		cmd.Help()
	},
}

var initCmd = &cobra.Command{
	Use:   "init [target directory]",
	Short: "Creates a basic qaz project",
	Run: func(cmd *cobra.Command, args []string) {

		// Print Banner
		banner.Print("qaz")
		fmt.Printf("\n--\n")

		var target string
		switch len(args) {
		case 0:
			target, _ = os.Getwd()
		default:
			target = args[0]
		}

		// Get Project & AWS Region
		project = getInput("-> Enter your Project name", "qaz-project")
		region = getInput("-> Enter AWS Region", "eu-west-1")

		// set target paths
		c := filepath.Join(target, "config.yml")
		t := filepath.Join(target, "templates")
		f := filepath.Join(target, "files")

		// Check if config file exists
		var overwrite string
		if _, err := os.Stat(c); err == nil {
			overwrite = getInput(
				fmt.Sprintf("%s [%s] already exist, Do you want to %s?(Y/N) ", colorString("->", "yellow"), c, colorString("Overwrite", "red")),
				"N",
			)

			if overwrite == "Y" {
				fmt.Println(fmt.Sprintf("%s Overwriting: [%s]..", colorString("->", "yellow"), c))
			}
		}

		// Create template file
		if overwrite != "N" {
			if err := ioutil.WriteFile(c, configTemplate(project, region), 0644); err != nil {
				fmt.Printf("%s Error, unable to create config.yml file: %s"+"\n", err, colorString("->", "red"))
				return
			}
		}

		// Create template folder
		for _, dir := range []string{t, f} {
			if err := os.Mkdir(dir, os.ModePerm); err != nil {
				fmt.Printf("%s [%s] folder not created: %s"+"\n--\n", colorString("->", "yellow"), dir, err)
				return
			}
		}

		fmt.Println("--")
	},
}

var generateCmd = &cobra.Command{
	Use:   "generate [stack]",
	Short: "Generates template from configuration values",
	Example: strings.Join([]string{
		"",
		"qaz generate -c config.yml -t vpc.yml",
		"qaz generate vpc -c config.yml",
	}, "\n"),
	Run: func(cmd *cobra.Command, args []string) {

		job.request = "generate"
		var s string
		var source string

		err := configReader(job.cfgSource)
		if err != nil {
			handleError(err)
			return
		}

		if job.tplSource != "" {
			s, source, err = getSource(job.tplSource)
			if err != nil {
				handleError(err)
				return
			}
		}

		if len(args) > 0 {
			s = args[0]
		}

		// check if stack exists in config
		if _, ok := stacks[s]; !ok {
			handleError(fmt.Errorf("Stack [%s] not found in config", s))
			return
		}

		if stacks[s].source == "" {
			stacks[s].source = source
		}

		name := fmt.Sprintf("%s-%s", project, s)
		Log(fmt.Sprintln("Generating a template for ", name), "debug")

		err = stacks[s].genTimeParser()
		if err != nil {
			handleError(err)
			return
		}
		fmt.Println(stacks[s].template)
	},
}

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploys stack(s) to AWS",
	Example: strings.Join([]string{
		"qaz deploy stack -c path/to/config",
		"qaz deploy -c path/to/config -t stack::s3//bucket/key",
		"qaz deploy -c path/to/config -t stack::path/to/template",
	}, "\n"),
	Run: func(cmd *cobra.Command, args []string) {

		job.request = "deploy"

		err := configReader(job.cfgSource)
		if err != nil {
			handleError(err)
			return
		}

		job.stacks = make(map[string]string)

		// Add job stacks based on templates Flags
		for _, src := range job.tplSources {
			s, source, err := getSource(src)
			if err != nil {
				handleError(err)
				return
			}
			job.stacks[s] = source
		}

		// Add all stacks with defined sources if all
		if job.all {
			for s, v := range stacks {
				// so flag values aren't overwritten
				if _, ok := job.stacks[s]; !ok {
					job.stacks[s] = v.source
				}
			}
		}

		// Add job stacks based on Args
		if len(args) > 0 && !job.all {
			for _, stk := range args {
				if _, ok := stacks[stk]; !ok {
					handleError(fmt.Errorf("Stack [%s] not found in conig", stk))
					return
				}
				job.stacks[stk] = stacks[stk].source
			}
		}

		for s, src := range job.stacks {
			if stacks[s].source == "" {
				stacks[s].source = src
			}
			if err := stacks[s].genTimeParser(); err != nil {
				handleError(err)
			} else {

				// Handle missing stacks
				if stacks[s] == nil {
					handleError(fmt.Errorf("Missing Stack in %s: [%s]", job.cfgSource, s))
					return
				}
			}
		}

		// Deploy Stacks
		DeployHandler()

	},
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Updates a given stack",
	Example: strings.Join([]string{
		"qaz update -c path/to/config -t stack::path/to/template",
		"qaz update -c path/to/config -t stack::s3//bucket/key",
		"qaz update -c path/to/config -t stack::http://someurl",
	}, "\n"),
	Run: func(cmd *cobra.Command, args []string) {

		job.request = "update"
		var s string
		var source string

		err := configReader(job.cfgSource)
		if err != nil {
			handleError(err)
			return
		}

		if job.tplSource != "" {
			s, source, err = getSource(job.tplSource)
			if err != nil {
				handleError(err)
				return
			}
		}

		if len(args) > 0 {
			s = args[0]
		}

		// check if stack exists in config
		if _, ok := stacks[s]; !ok {
			handleError(fmt.Errorf("Stack [%s] not found in config", s))
			return
		}

		if stacks[s].source == "" {
			stacks[s].source = source
		}

		err = stacks[s].genTimeParser()
		if err != nil {
			handleError(err)
			return
		}

		// Handle missing stacks
		if stacks[s] == nil {
			handleError(fmt.Errorf("Missing Stack in %s: [%s]", job.cfgSource, s))
			return
		}

		if err := stacks[s].update(); err != nil {
			handleError(err)
			return
		}

	},
}

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Validates Cloudformation Templates",
	Example: strings.Join([]string{
		"qaz check -c path/to/config.yml -t path/to/template -c path/to/config",
		"qaz check -c path/to/config.yml -t stack::http://someurl.example",
		"qaz check -c path/to/config.yml -t stack::s3://bucket/key",
	}, "\n"),
	Run: func(cmd *cobra.Command, args []string) {

		job.request = "validate"
		var s string
		var source string

		err := configReader(job.cfgSource)
		if err != nil {
			handleError(err)
			return
		}

		if job.tplSource != "" {
			s, source, err = getSource(job.tplSource)
			if err != nil {
				handleError(err)
				return
			}
		}

		if len(args) > 0 {
			s = args[0]
		}

		// check if stack exists in config
		if _, ok := stacks[s]; !ok {
			handleError(fmt.Errorf("Stack [%s] not found in config", s))
			return
		}

		if stacks[s].source == "" {
			stacks[s].source = source
		}

		name := fmt.Sprintf("%s-%s", config.Project, s)
		fmt.Println("Validating template for", name)

		if err = stacks[s].genTimeParser(); err != nil {
			handleError(err)
			return
		}

		if err := stacks[s].check(); err != nil {
			handleError(err)
			return
		}
	},
}

var terminateCmd = &cobra.Command{
	Use:   "terminate [stacks]",
	Short: "Terminates stacks",
	Run: func(cmd *cobra.Command, args []string) {

		job.request = "terminate"

		if !job.all {
			job.stacks = make(map[string]string)
			for _, stk := range args {
				job.stacks[stk] = ""
			}

			if len(job.stacks) == 0 {
				Log("No stack specified for termination", level.warn)
				return
			}
		}

		err := configReader(job.cfgSource)
		if err != nil {
			handleError(err)
			return
		}

		// Terminate Stacks
		TerminateHandler()
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Prints status of deployed/un-deployed stacks",
	Run: func(cmd *cobra.Command, args []string) {

		job.request = "status"

		err := configReader(job.cfgSource)
		if err != nil {
			handleError(err)
			return
		}

		for _, v := range stacks {
			wg.Add(1)
			go func(s *stack) {
				if err := s.status(); err != nil {
					handleError(err)
				}
				wg.Done()
			}(v)

		}
		wg.Wait()
	},
}

var outputsCmd = &cobra.Command{
	Use:     "outputs [stack]",
	Short:   "Prints stack outputs",
	Example: "qaz outputs vpc subnets --config path/to/config",
	Run: func(cmd *cobra.Command, args []string) {

		job.request = "outputs"

		if len(args) < 1 {
			fmt.Println("Please specify stack(s) to check, For details try --> qaz outputs --help")
			return
		}

		err := configReader(job.cfgSource)
		if err != nil {
			handleError(err)
			return
		}

		for _, s := range args {
			// check if stack exists
			if _, ok := stacks[s]; !ok {
				handleError(fmt.Errorf("%s: does not Exist in Config", s))
				continue
			}

			wg.Add(1)
			go func(s string) {
				if err := stacks[s].outputs(); err != nil {
					handleError(err)
					wg.Done()
					return
				}

				for _, i := range stacks[s].output.Stacks {
					m, err := json.MarshalIndent(i.Outputs, "", "  ")
					if err != nil {
						handleError(err)

					}

					fmt.Println(string(m))
				}

				wg.Done()
			}(s)
		}
		wg.Wait()

	},
}

var exportsCmd = &cobra.Command{
	Use:     "exports",
	Short:   "Prints stack exports",
	Example: "qaz exports",
	Run: func(cmd *cobra.Command, args []string) {

		job.request = "exports"

		sess, err := manager.GetSess(job.profile)
		if err != nil {
			handleError(err)
			return
		}

		Exports(sess)

	},
}

var invokeCmd = &cobra.Command{
	Use:   "invoke",
	Short: "Invoke AWS Lambda Functions",
	Run: func(cmd *cobra.Command, args []string) {
		job.request = "lambda_invoke"

		if len(args) < 1 {
			fmt.Println("No Lambda Function specified")
			return
		}

		sess, err := manager.GetSess(job.profile)
		if err != nil {
			handleError(err)
			return
		}

		f := function{name: args[0]}

		if job.funcEvent != "" {
			f.payload = []byte(job.funcEvent)
		}

		if err := f.Invoke(sess); err != nil {
			if strings.Contains(err.Error(), "Unhandled") {
				handleError(fmt.Errorf("Unhandled Exception: Potential Issue with Lambda Function Logic for %s...\n", f.name))
			}
			handleError(err)
			return
		}

		fmt.Println(f.response)

	},
}

var policyCmd = &cobra.Command{
	Use:     "set-policy",
	Short:   "Set Stack Policies based on configured value",
	Example: "qaz set-policy <stack name>",
	Run: func(cmd *cobra.Command, args []string) {

		job.request = "set-policy"

		if len(args) == 0 {
			handleError(fmt.Errorf("Please specify stack name..."))
			return
		}

		err := configReader(job.cfgSource)
		if err != nil {
			handleError(err)
			return
		}

		for _, s := range args {
			wg.Add(1)
			go func(s string) {

				if _, ok := stacks[s]; !ok {
					handleError(fmt.Errorf("Stack [%s] not found in config", s))

				} else {
					if err := stacks[s].stackPolicy(); err != nil {
						handleError(err)
					}
				}

				wg.Done()
				return

			}(s)
		}

		wg.Wait()

	},
}
