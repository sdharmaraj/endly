package exec_test

import (
	"github.com/stretchr/testify/assert"
	"github.com/viant/endly"
	"github.com/viant/endly/model"
	"github.com/viant/endly/system/exec"
	"github.com/viant/endly/util"
	"github.com/viant/toolbox/url"
	"log"
	"os"
	"testing"
)

func TestNewExecService(t *testing.T) {

	var useCases = []struct {
		description string
		baseDir     string
		target      *url.Resource
		expected    *model.OperatingSystem
	}{
		{
			description: "open new session on linux",
			baseDir:     "test/open/linux",
			target:      url.NewResource("ssh://127.0.0.1:22/etc"),
			expected:    &model.OperatingSystem{Name: "ubuntu", Architecture: "x64", Hardware: "x86_64", Version: "17.04", System: "linux"},
		},
		{
			description: "open new session on osx",
			baseDir:     "test/open/darwin",
			target:      url.NewResource("ssh://127.0.0.1:22/etc"),
			expected:    &model.OperatingSystem{Name: "macosx", Architecture: "x64", Hardware: "x86_64", Version: "10.12.6", System: "darwin"},
		},
	}

	manager := endly.New()
	for _, useCase := range useCases {
		context, err := exec.NewSSHReplayContext(manager, useCase.target, useCase.baseDir)
		defer context.Close()
		if assert.Nil(t, err) {
			var target = useCase.target
			actual := exec.OperatingSystem(context, target.Host())
			if assert.NotNil(t, actual) {
				expected := useCase.expected
				assert.Equal(t, expected.Name, actual.Name, "os.name")
				assert.Equal(t, expected.Version, actual.Version, "os.version")
				assert.Equal(t, expected.Hardware, actual.Hardware, "os.hardware")
				assert.Equal(t, expected.System, actual.System, "os.system")
			}
		}
	}

}

func Test_NoTransientSession(t *testing.T) {
	manager := endly.New()
	var credentials, err = util.GetDummyCredential()
	if err != nil {
		log.Fatal(err)
	}
	target := url.NewResource("ssh://127.0.0.1", credentials)
	context, err := exec.NewSSHReplayContext(manager, target, "test/session/context")
	if err != nil {
		log.Fatal(err)
	}
	response, err := manager.Run(context, exec.NewOpenSessionRequest(target, []string{"/usr/local/bin"}, map[string]string{"M2_HOME": "/users/test/.m2/"}, false, "/"))
	if err != nil {
		log.Fatal(err)
	}
	openResponse := response.(*exec.OpenSessionResponse)

	sessions := exec.TerminalSessions(context)
	assert.True(t, sessions.Has(openResponse.SessionID))
	log.Print(openResponse.SessionID)
	context.Close()
	assert.False(t, sessions.Has(openResponse.SessionID))
}

func Test_TransientSession(t *testing.T) {
	manager := endly.New()
	var credentials, err = util.GetDummyCredential()
	if err != nil {
		log.Fatal(err)
	}
	target := url.NewResource("ssh://127.0.0.1", credentials)
	context, err := exec.NewSSHReplayContext(manager, target, "test/session/transient")
	if err != nil {
		log.Fatal(err)
	}
	response, err := manager.Run(context, exec.NewOpenSessionRequest(target, []string{"/usr/local/bin"}, map[string]string{"M2_HOME": "/users/test/.m2/"}, true, "/"))
	if err != nil {
		log.Fatal(err)
	}
	openResponse := response.(*exec.OpenSessionResponse)
	sessions := exec.TerminalSessions(context)
	assert.True(t, sessions.Has(openResponse.SessionID))
	log.Print(openResponse.SessionID)
	context.Close()
	assert.False(t, sessions.Has(openResponse.SessionID))

}

func TestRunCommand(t *testing.T) {

	{ //simple command
		manager := endly.New()
		var credentials, err = util.GetDummyCredential()
		if err != nil {
			log.Fatal(err)
		}
		target := url.NewResource("ssh://127.0.0.1", credentials)
		context, err := exec.NewSSHReplayContext(manager, target, "test/run/simple")
		if err != nil {
			log.Fatal(err)
		}

		resp, err := manager.Run(context, exec.NewRunRequest(target, false, "whoami"))
		if !assert.Nil(t, err) {
			log.Fatal(err.Error())
		}
		runResponse := resp.(*exec.RunResponse)
		assert.EqualValues(t, os.Getenv("USER"), runResponse.Output)
	}

	{

		manager := endly.New()
		var credentials, err = util.GetDummyCredential()
		if err != nil {
			log.Fatal(err)
		}
		target := url.NewResource("ssh://127.0.0.1", credentials)
		context, err := exec.NewSSHReplayContext(manager, target, "test/run/conditional")
		if err != nil {
			log.Fatal(err)
		}
		var runRequest = exec.NewRunRequest(target, true, "whoami", "$stdout:/root/? echo 'hello root'")
		var runResponse = &exec.RunResponse{}
		err = endly.Run(context, runRequest, runResponse)
		if !assert.Nil(t, err) {
			log.Fatal(err.Error())
		}
		assert.NotNil(t, "hello root", runResponse.Stdout(1))

	}
}
