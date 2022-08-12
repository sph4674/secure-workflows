package main

import (
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
)

const (
	HardenRunnerActionPathWithTag = "step-security/harden-runner@v1"
	HardenRunnerActionPath        = "step-security/harden-runner"
	HardenRunnerActionName        = "Harden Runner"
)

func SecureWorkflow(queryStringParams map[string]string, inputYaml string, svc dynamodbiface.DynamoDBAPI) (*SecureWorkflowReponse, error) {
	pinActions, addHardenRunner, addPermissions, addProjectComment := true, true, true, true
	pinnedActions, addedHardenRunner, addedPermissions := false, false, false
	ignoreMissingKBs := false

	if queryStringParams["pinActions"] == "false" {
		pinActions = false
	}

	if queryStringParams["addHardenRunner"] == "false" {
		addHardenRunner = false
	}

	if queryStringParams["addPermissions"] == "false" {
		addPermissions = false
	}

	if queryStringParams["ignoreMissingKBs"] == "true" {
		ignoreMissingKBs = true
	}

	if queryStringParams["addProjectComment"] == "false" {
		addProjectComment = false
	}

	secureWorkflowReponse := &SecureWorkflowReponse{FinalOutput: inputYaml, OriginalInput: inputYaml}
	var err error
	if addPermissions {
		secureWorkflowReponse, err = AddJobLevelPermissions(secureWorkflowReponse.FinalOutput)
		secureWorkflowReponse.OriginalInput = inputYaml
		if err != nil {
			return nil, err
		} else {
			if !secureWorkflowReponse.HasErrors || shouldAddWorkflowLevelPermissions(secureWorkflowReponse.JobErrors) {
				secureWorkflowReponse.FinalOutput, err = AddWorkflowLevelPermissions(secureWorkflowReponse.FinalOutput, addProjectComment)
				if err != nil {
					secureWorkflowReponse.HasErrors = true
				} else {
					// reset the error
					// this is done because workflow perms have been added
					// only job errors were that perms already existed
					secureWorkflowReponse.HasErrors = false
				}
			}
			if len(secureWorkflowReponse.MissingActions) > 0 && !ignoreMissingKBs {
				StoreMissingActions(secureWorkflowReponse.MissingActions, svc)
			}
		}
		// if there are no errors, then we must have added perms
		// if there are already perms at workflow level, that is treated as an error condition
		addedPermissions = !secureWorkflowReponse.HasErrors
	}

	if addHardenRunner {
		secureWorkflowReponse.FinalOutput, addedHardenRunner, _ = AddAction(secureWorkflowReponse.FinalOutput, HardenRunnerActionPathWithTag)
	}

	if pinActions {
		pinnedAction, pinnedDocker := false, false
		secureWorkflowReponse.FinalOutput, pinnedAction, _ = PinActions(secureWorkflowReponse.FinalOutput)
		secureWorkflowReponse.FinalOutput, pinnedDocker, _ = PinDocker(secureWorkflowReponse.FinalOutput)
		pinnedActions = pinnedAction || pinnedDocker
	}

	// Setting appropriate flags
	secureWorkflowReponse.PinnedActions = pinnedActions
	secureWorkflowReponse.AddedHardenRunner = addedHardenRunner
	secureWorkflowReponse.AddedPermissions = addedPermissions
	return secureWorkflowReponse, nil
}

func shouldAddWorkflowLevelPermissions(jobErrors []JobError) bool {
	if len(jobErrors) == 0 {
		// if there are no job errors, there must have been workflow level errors,
		// else this method would not have been called
		// so we do not add workflow level permissions
		return false
	}
	for _, jobError := range jobErrors {
		for _, eachJobError := range jobError.Errors {
			if eachJobError != errorAlreadyHasPermissions {
				// if any of the errors is not errorAlreadyHasPermissions
				// we do not add workflow level permissions
				return false
			}
		}
	}

	// if there were job errors and all of them were errorAlreadyHasPermissions
	// we can add workflow level permissions
	return true
}
