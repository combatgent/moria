package moria

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/fatih/color"
	env "github.com/joho/godotenv"
)

var pError = color.New(color.FgRed).PrintfFunc()
var pWarning = color.New(color.FgYellow).PrintfFunc()
var pSuccess = color.New(color.FgGreen).PrintfFunc()
var pSuccessInline = color.New(color.FgGreen).SprintfFunc()
var pInfo = color.New(color.FgBlue).PrintfFunc()
var pMethod = color.New(color.FgWhite, color.Bold).SprintfFunc()
var pBold = color.New(color.FgHiWhite, color.Bold).SprintfFunc()

// required
var required = [...]string{
	"GO_ENV",
	"ETCD_USERNAME",
	"ETCD_PASSWORD",
	"ETCD_CA_STRING",
	"ETCD_CLIENT_PEERS"}

var optional = [...]string{
	"NAMESPACE",
	"HEROKU_KEY",
	"CIRCLECI_TOKEN",
	"CIRCLECI_USERNAME",
}

// Environment loads ands checks for missing environment values on boot
func Environment() string {
	loadEnvironment()
	checkRequired()
	checkOptional()
	environment := os.Getenv("GO_ENV")
	return environment
}

//
func loadEnvironment() {
	errEnv := env.Load()
	if errEnv != nil {
		fmt.Println("ERR IF NO ENV:", errEnv.Error())
	}
	errAuxEnv := checkAuxillaryEnvironmentPath()
	if errEnv != nil && errAuxEnv != nil {
		noEnvironmentFound()
	}
}

//
func checkAuxillaryEnvironmentPath() error {
	path := os.Getenv("ENV_PATH")
	if path != "" {
		loadPath(path)
	}
	return errors.New("no aux env found")
}

func loadPath(path string) error {
	err := env.Load(path)
	if err != nil {
		invalidAuxEnvPath()
	}
	return err
}

// checkRequired ensures necessary config vars are present
// Minimum Required Vars Include:
//  * GO_ENV
//  * ETCD_USERNAME
//  * ETCD_PASSWORD
//  * ETCD_HOST
//  * ETCD_PORT
//  * ETCD_CA_PATH
//  * ETCD_PEERS
func checkRequired() {
	defer func() {
		if perr := recover(); perr != nil {
			var ok bool
			perr, ok = perr.(error)
			if !ok {
				fmt.Errorf("Panicking: %v", perr)
			}
		}
	}()
	for _, req := range required {
		if os.Getenv(req) == "" {
			missingRequiredValue(req)
		}
	}
}

// checkOptional ensures ancillary config vars are present for CI & deployment
// Currenty Supported Optional Vars Include:
//  * HEROKU_KEY
//  * CIRCLECI_TOKEN
//  * CIRCLECI_USERNAME
func checkOptional() {
	defer func() {
		if perr := recover(); perr != nil {
			var ok bool
			perr, ok = perr.(error)
			if !ok {
				fmt.Errorf("Panicking: %v", perr)
			}
		}
	}()
	for _, req := range optional {
		if os.Getenv(req) == "" {
			missingOptionalValue(req)
		}
	}
}

// WARNINGS
func missingOptionalValue(v string) {
	log.Panicf(
		color.YellowString(
			`*************************** WARNING *******************************
  UNABLE TO LOCATE/LOAD THE %s ENVIRONMENT CONFIGURATION VARIABLE`), v)
}

// FATAL ERRORS

func missingRequiredValue(v string) {
	color.Set(color.FgRed)
	log.Panicf(
		`*************************** ERROR *******************************
  UNABLE TO LOCATE/LOAD THE %s ENVIRONMENT CONFIGURATION VARIABLE`, v)

}

func invalidAuxEnvPath() {
	color.Set(color.FgRed)
	log.Panicln(
		`**************************** ERROR ********************************
  INVALID PATH SUPPLIED FOR AUX ENVIRONMENT PATH CONFIGURATION VARIABLE`)
}

//
func noEnvironmentFound() {
	color.Set(color.FgRed)
	log.Panicln(
		`*************************** ERROR *******************************
  UNABLE TO LOCATE/LOAD REQUIRED ENVIRONMENT CONFIGURATION VARIABLES`)
}
