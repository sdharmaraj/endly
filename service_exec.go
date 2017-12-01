package endly

import (
	"fmt"
	"github.com/viant/toolbox"
	"github.com/viant/toolbox/cred"
	"github.com/viant/toolbox/ssh"
	"github.com/viant/toolbox/url"
	"path"
	"sort"
	"strings"
	"sync"
)

//ExecServiceID represent system executor service id
const ExecServiceID = "exec"

const sudoCredentialKey = "**sudo**"

//ExecutionStartEvent represents an execution event start
type ExecutionStartEvent struct {
	SessionID string
	Stdin     string
}

//ExecutionEndEvent represents an execution event end
type ExecutionEndEvent struct {
	SessionID string
	Stdout    string
	Error     string
}

type execService struct {
	*AbstractService
	mutex               *sync.RWMutex
	credentialPasswords map[string]string
}

func (s *execService) open(context *Context, request *OpenSessionRequest) (*OpenSessionResponse, error) {
	var clientSession, err = s.openSession(context, request)
	if err != nil {
		return nil, err
	}
	return &OpenSessionResponse{
		SessionID: clientSession.ID,
	}, nil
}

func (s *execService) openSshService(context *Context, request *OpenSessionRequest) (ssh.Service, error) {
	if request.ReplayService != nil {
		return request.ReplayService, nil
	}
	target, err := context.ExpandResource(request.Target)
	if err != nil {
		return nil, err
	}
	var authConfig = &cred.Config{}
	if target.Credential != "" {
		err = authConfig.Load(target.Credential)
		if err != nil {
			return nil, err
		}
	}
	hostname, port := getHostAndSSHPort(target)
	return ssh.NewService(hostname, port, authConfig)
}

func (s *execService) openSession(context *Context, request *OpenSessionRequest) (*SystemTerminalSession, error) {
	s.Mutex().Lock()
	defer s.Mutex().Unlock()
	target, err := context.ExpandResource(request.Target)
	if err != nil {
		return nil, err
	}
	if !(target.ParsedURL.Scheme == "ssh" || target.ParsedURL.Scheme == "scp" || target.ParsedURL.Scheme == "file") {
		return nil, fmt.Errorf("Failed to open sessionName: invalid schema: %v in url: %v", target.ParsedURL.Scheme, target.URL)
	}
	sessions := context.TerminalSessions()

	var replayCommands *ssh.ReplayCommands
	if request.CommandsBasedir != "" {
		replayCommands, err = ssh.NewReplayCommands(request.CommandsBasedir)
		if err != nil {
			return nil, err
		}
	}

	var sessionName = target.Host()
	if sessions.Has(sessionName) {
		session := sessions[sessionName]
		err = s.changeDirectory(context, session, nil, target.ParsedURL.Path)
		for k, v := range request.Env {
			session.envVariables[k] = v
		}

		return sessions[sessionName], err
	}
	sshService, err := s.openSshService(context, request)
	if replayCommands != nil {
		err = replayCommands.Enable(sshService)
		if err != nil {
			return nil, err
		}
		context.Deffer(func() {
			replayCommands.Store()
		})
	}
	session, err := NewSystemTerminalSession(sessionName, sshService)
	if err != nil {
		return nil, err
	}
	if !request.Transient {
		context.Deffer(func() {
			sshService.Close()
		})
	}

	session.MultiCommandSession, err = session.Service.OpenMultiCommandSession(request.Config)
	if err != nil {
		return nil, err
	}
	if !request.Transient {
		context.Deffer(func() {
			session.MultiCommandSession.Close()
		})
	}
	err = s.changeDirectory(context, session, nil, target.ParsedURL.Path)
	if err != nil {
		return nil, err
	}
	for k, v := range request.Env {
		session.envVariables[k] = v
	}
	sessions[sessionName] = session
	session.OperatingSystem, err = s.detectOperatingSystem(session)
	if err != nil {
		return nil, err
	}
	return session, nil
}

func getHostAndSSHPort(target *url.Resource) (string, int) {
	port := toolbox.AsInt(target.ParsedURL.Port())
	if port == 0 {
		port = 22
	}
	hostname := target.ParsedURL.Hostname()
	if hostname == "" {
		hostname = "127.0.0.1"
	}
	return hostname, port
}

func (s *execService) setEnvVariables(context *Context, session *SystemTerminalSession, env map[string]string) error {
	for k, v := range env {
		err := s.setEnvVariable(context, session, k, v)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *execService) setEnvVariable(context *Context, session *SystemTerminalSession, name, newValue string) error {
	newValue = context.Expand(newValue)

	if actual, has := session.envVariables[name]; has {
		if newValue == actual  {
			return nil
		}
	}
	session.envVariables[name] = newValue
	return s.rumCommandTemplate(context, session, "export %v='%v'", name, newValue)
}

func (s *execService) changeDirectory(context *Context, session *SystemTerminalSession, commandInfo *CommandResponse, directory string) error {
	if directory == "" {
		return nil
	}
	parent, name := path.Split(directory)
	if path.Ext(name) != "" {
		directory = parent
	}
	if session.currentDirectory == directory {
		return nil
	}
	session.currentDirectory = directory
	return s.rumCommandTemplate(context, session, "cd %v", directory)
}

func (s *execService) rumCommandTemplate(context *Context, session *SystemTerminalSession, commandTemplate string, arguments ...interface{}) error {
	command := fmt.Sprintf(commandTemplate, arguments...)
	var executionStartEvent = &ExecutionStartEvent{SessionID: session.ID, Stdin: command}
	startEvent := s.Begin(context, executionStartEvent, Pairs("value", executionStartEvent), Info)
	stdout, err := session.Run(command, 1000)
	var executionEndEvent = &ExecutionEndEvent{
		SessionID: session.ID,
		Stdout:    stdout,
	}
	s.End(context)(startEvent, Pairs("value", executionEndEvent))
	if err != nil {
		executionEndEvent.Error = fmt.Sprintf("%v", err)
	}
	if err != nil {
		return err
	}
	return nil
}

func (s *execService) applyCommandOptions(context *Context, options *ExecutionOptions, session *SystemTerminalSession, info *CommandResponse) error {
	operatingSystem := session.OperatingSystem
	if options == nil {
		return nil
	}
	if len(options.SystemPaths) > 0 {
		operatingSystem.Path.Push(options.SystemPaths...)
	}
	s.setEnvVariables(context, session, options.Env)
	if options.Directory != "" {
		directory := context.Expand(options.Directory)
		err := s.changeDirectory(context, session, info, directory)
		if err != nil {
			return err
		}
	}
	return nil
}

func match(stdout string, candidates ...string) string {
	if len(candidates) == 0 {
		return ""
	}
	for _, candidate := range candidates {
		if strings.Contains(stdout, candidate) {
			return candidate
		}
	}
	return ""
}

//TODO caching this
func (s *execService) credentialPassword(credentialPath string) (string, error) {
	s.mutex.RLock()
	password, has := s.credentialPasswords[credentialPath]
	s.mutex.RUnlock()
	if has {
		return password, nil
	}
	if credentialPath != "" && toolbox.FileExists(credentialPath) {
		credential, err := cred.NewConfig(credentialPath)
		if err != nil {
			return "", err
		}
		s.mutex.Lock()
		password = credential.Password
		s.credentialPasswords[credentialPath] = password
		s.mutex.Unlock()

	}
	return password, nil
}

func (s *execService) credentialsToSecure(credentials map[string]string) (map[string]string, error) {
	var secure = make(map[string]string)
	if len(credentials) > 0 {
		for k, v := range credentials {
			secure[k] = v
			var credential, err = s.credentialPassword(v)
			if err != nil {
				return nil, err
			}
			secure[k] = credential
		}
	}
	return secure, nil
}

func (s *execService) executeCommand(context *Context, session *SystemTerminalSession, execution *Execution, options *ExecutionOptions, response *CommandResponse, request *ManagedCommandRequest) error {
	command := context.Expand(execution.Command)

	terminators := getTerminators(options, session, execution)

	var cmd = command
	if len(execution.Credentials) > 0 {
		secure, err := s.credentialsToSecure(execution.Credentials)
		if err != nil {
			return fmt.Errorf("Failed to run commend: %v, invalid credential: %v %v ", command, execution.Credentials, err)
		}
		var keys = toolbox.MapKeysToStringSlice(secure)
		sort.Strings(keys)
		for _, key := range keys {
			cmd = strings.Replace(cmd, key, secure[key], len(command))
		}
	}

	var executionStartEvent = &ExecutionStartEvent{SessionID: session.ID, Stdin: command}
	startEvent := s.Begin(context, executionStartEvent, Pairs("value", executionStartEvent), Info)
	stdout, err := session.Run(cmd, options.TimeoutMs, terminators...)
	var executionEndEvent = &ExecutionEndEvent{
		SessionID: session.ID,
		Stdout:    stdout,
	}

	if err != nil {
		executionEndEvent.Error = fmt.Sprintf("%v", err)
	}
	s.End(context)(startEvent, Pairs("value", executionEndEvent))

	response.Add(NewCommandLog(command, stdout, err))
	if err != nil {
		return err
	}

	errorMatch := match(stdout, execution.Error...)
	if errorMatch != "" {
		return fmt.Errorf("Encounter error fragment: (%v) execution (%v); ouput: (%v), %v", errorMatch, execution.Command, stdout, options.Directory)
	}
	if len(execution.Success) > 0 {
		sucessMatch := match(stdout, execution.Success...)
		if sucessMatch == "" {
			return fmt.Errorf("Fail to match any fragment: (%v) execution (%v); ouput: (%v), %v", strings.Join(execution.Success, ","), execution.Command, stdout, options.Directory)
		}
	}
	err = execution.Extraction.Extract(context, response.Extracted, strings.Split(stdout, "\n")...)
	if err != nil {
		return err
	}

	if len(stdout) > 0 {
		for _, execution := range request.ManagedCommand.Executions {
			if execution.MatchOutput != "" && strings.Contains(stdout, execution.MatchOutput) {
				return s.executeCommand(context, session, execution, options, response, request)
			}
		}
	}
	return nil
}
func getTerminators(options *ExecutionOptions, session *SystemTerminalSession, execution *Execution) []string {
	var terminators = append([]string{}, options.Terminators...)
	terminators = append(terminators, "$ ")
	superUserPrompt := string(strings.Replace(session.ShellPrompt(), "$", "#", 1))
	if strings.Contains(superUserPrompt, "bash") {
		superUserPrompt = string(superUserPrompt[2:])
	}
	terminators = append(terminators, superUserPrompt)
	terminators = append(terminators, execution.Error...)
	return terminators
}

func (s *execService) runCommands(context *Context, request *ManagedCommandRequest) (*CommandResponse, error) {
	err := request.Validate()
	if err != nil {
		return nil, err
	}
	target, err := context.ExpandResource(request.Target)
	if err != nil {
		return nil, err
	}
	session, err := s.openSession(context, &OpenSessionRequest{Target: target})
	if err != nil {
		return nil, err
	}

	var options = request.ManagedCommand.Options
	if options == nil {
		options = NewExecutionOptions()
	}
	response := NewCommandResponse(session.ID)
	err = s.applyCommandOptions(context, options, session, response)

	if err != nil {
		return nil, err
	}

	operatingSystem := session.OperatingSystem
	err = s.setEnvVariable(context, session, "PATH", operatingSystem.Path.EnvValue())
	if err != nil {
		return nil, err
	}

	response = NewCommandResponse(session.ID)
	for _, execution := range request.ManagedCommand.Executions {
		var command = context.Expand(execution.Command)
		if execution.MatchOutput != "" {
			continue
		}
		if strings.HasPrefix(command, "cd ") {
			if !strings.Contains(command, "&&") {
				var directory = strings.TrimSpace(string(command[3:]))
				err = s.changeDirectory(context, session, response, directory)
				continue
			}
			session.currentDirectory = "" //reset path
		}
		if strings.HasPrefix(command, "export ") {
			if !strings.Contains(command, "&&") {
				envVariable := string(command[7:])
				keyValuePair := strings.Split(envVariable, "=")
				if len(keyValuePair) == 2 {
					key := strings.TrimSpace(keyValuePair[0])
					value := strings.TrimSpace(keyValuePair[1])
					value = strings.Trim(value, "'\"")
					err = s.setEnvVariable(context, session, key, value)
					continue
				}
			}
			session.envVariables = make(map[string]string) //reset env variables
		}
		err = s.executeCommand(context, session, execution, options, response, request)
		if err != nil {
			return nil, err
		}

	}
	return response, nil
}

func (s *execService) closeSession(context *Context, request *CloseSessionRequest) (*CloseSessionResponse, error) {
	clientSessions := context.TerminalSessions()
	if session, has := clientSessions[request.SessionID]; has {
		session.Close()
	}
	if connection, has := clientSessions[request.SessionID]; has {
		connection.Close()
	}
	return &CloseSessionResponse{
		SessionID: request.SessionID,
	}, nil
}

//Run runs action for passed in request.
func (s *execService) Run(context *Context, request interface{}) *ServiceResponse {
	startEvent := s.Begin(context, request, Pairs("request", request))
	var response = &ServiceResponse{Status: "ok"}
	defer s.End(context)(startEvent, Pairs("response", response))
	var err error
	switch actualRequest := request.(type) {
	case *CommandRequest:
		var mangedCommandRequest = actualRequest.AsManagedCommandRequest()
		if actualRequest.SuperUser {
			superCommandRequest := superUserCommandRequest{
				Target:        actualRequest.Target,
				MangedCommand: mangedCommandRequest.ManagedCommand,
			}
			mangedCommandRequest, err = superCommandRequest.AsCommandRequest(context)
		}
		if err == nil {
			response.Response, err = s.runCommands(context, mangedCommandRequest)
		}
		if err != nil {
			response.Error = fmt.Sprintf("Failed to run command: %v, %v", actualRequest, err)
		}

	case *OpenSessionRequest:
		response.Response, err = s.open(context, actualRequest)
		if err != nil {
			response.Error = fmt.Sprintf("Failed to open session: %v, %v", actualRequest.Target, err)
		}
	case *ManagedCommandRequest:
		response.Response, err = s.runCommands(context, actualRequest)
		if err != nil {
			response.Error = fmt.Sprintf("Failed to run command: %v, %v", actualRequest.ManagedCommand, err)
		}
	case *superUserCommandRequest:
		commandRequest, err := actualRequest.AsCommandRequest(context)
		if err == nil {
			response.Response, err = s.runCommands(context, commandRequest)
		}
		if err != nil {
			response.Error = fmt.Sprintf("%v", err)
		}

	case *CloseSessionRequest:
		response.Response, err = s.closeSession(context, actualRequest)
		if err != nil {
			response.Error = fmt.Sprintf("Failed to close session: %v, %v", actualRequest.SessionID, err)
		}

	default:
		response.Error = fmt.Sprintf("Unsupported request type: %T", request)
	}

	if response.Error != "" {
		response.Status = "error"
	}
	return response
}

//NewRequest creates a new request for passed in action, the following is supported: open,close,command,managedCommand
func (s *execService) NewRequest(action string) (interface{}, error) {
	switch action {
	case "open":
		return &OpenSessionRequest{}, nil
	case "managed-command", "managedCommand":
		return &ManagedCommandRequest{}, nil
	case "command":
		return &CommandRequest{}, nil
	case "close":
		return &CloseSessionRequest{}, nil

	}
	return nil, fmt.Errorf("Unsupported action: %v", action)
}

func (s *execService) detectOperatingSystem(session *SystemTerminalSession) (*OperatingSystem, error) {
	operatingSystem := &OperatingSystem{
		Path: &SystemPath{
			SystemPath: make([]string, 0),
			Path:       make([]string, 0),
			index:      make(map[string]bool),
		},
	}

	varsionCheckCommand := "lsb_release -a"
	if session.MultiCommandSession.System() == "darwin" {
		varsionCheckCommand = "sw_vers"
	}
	output, err := session.Run(varsionCheckCommand, 0)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(output, "\r\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.Contains(line, "amd64") || strings.Contains(line, "x86_64") {
			operatingSystem.Architecture = "amd64"
		}
		pair := strings.Split(line, ":")
		if len(pair) != 2 {
			continue
		}

		var key = strings.Replace(strings.ToLower(pair[0]), " ", "", len(pair[0]))
		var val = strings.Replace(strings.Trim(pair[1], " \t\r"), " ", "", len(line))
		switch key {
		case "productname", "distributorid":
			operatingSystem.Name = strings.ToLower(val)
		case "productversion", "release":
			operatingSystem.Version = strings.ToLower(val)
		}

	}
	operatingSystem.Hardware, err = session.Run("uname -m", 0)
	if err != nil {
		return nil, err
	}
	if strings.Contains(operatingSystem.Hardware, "amd64") || strings.Contains(operatingSystem.Hardware, "x86_64") {
		operatingSystem.Architecture = "amd64"
	}

	operatingSystem.System = session.System()
	output, err = session.Run("echo $PATH", 0)
	if err != nil {
		return nil, err
	}
	lines = strings.Split(output, "\r\n")
	for i := 0; i < len(lines); i++ {
		var line = lines[i]
		if !strings.Contains(line, ":")  || !strings.Contains(line, "/") {
			continue
		}
		operatingSystem.Path.SystemPath = strings.Split(line, ":")
		break

	}
	return operatingSystem, nil
}

//NewExecService creates a new execution service
func NewExecService() Service {
	var result = &execService{
		mutex:               &sync.RWMutex{},
		credentialPasswords: make(map[string]string),
		AbstractService:     NewAbstractService(ExecServiceID),
	}
	result.AbstractService.Service = result
	return result
}

//superUserCommandRequest represents a super user command,
type superUserCommandRequest struct {
	Target        *url.Resource   //target destination where to run a command.
	MangedCommand *ManagedCommand //managed command
}

//AsCommandRequest returns ManagedCommandRequest
func (r *superUserCommandRequest) AsCommandRequest(context *Context) (*ManagedCommandRequest, error) {
	target, err := context.ExpandResource(r.Target)
	if err != nil {
		return nil, err
	}
	var result = &ManagedCommandRequest{
		Target: target,
		ManagedCommand: &ManagedCommand{
			Executions: make([]*Execution, 0),
		},
	}
	var executionOptions = &ExecutionOptions{
		Terminators: []string{"Password"},
	}
	if r.MangedCommand.Options != nil {
		executionOptions.Terminators = append(executionOptions.Terminators, r.MangedCommand.Options.Terminators...)
		executionOptions.TimeoutMs = r.MangedCommand.Options.TimeoutMs
		executionOptions.Directory = r.MangedCommand.Options.Directory
		executionOptions.SystemPaths = r.MangedCommand.Options.SystemPaths
	}
	result.ManagedCommand.Options = executionOptions
	var errors = make([]string, 0)
	var extractions = make([]*DataExtraction, 0)

	var credentials = make(map[string]string)

	for _, execution := range r.MangedCommand.Executions {
		if execution.Command == "" {
			continue
		}
		if len(execution.Credentials) > 0 {
			for k, v := range execution.Credentials {
				credentials[k] = v
			}
		}
		sudo := ""
		if len(execution.Command) > 1 {
			sudo = "sudo "
		}
		newExecution := &Execution{
			Command:     sudo + execution.Command,
			Error:       execution.Error,
			Extraction:  execution.Extraction,
			Success:     execution.Success,
			MatchOutput: execution.MatchOutput,
			Credentials: execution.Credentials,
		}
		if len(execution.Error) > 0 {
			errors = append(errors, execution.Error...)
		}
		if len(execution.Extraction) > 0 {
			extractions = append(extractions, execution.Extraction...)
		}
		result.ManagedCommand.Executions = append(result.ManagedCommand.Executions, newExecution)
	}

	if target.Credential == "" {
		return nil, fmt.Errorf("Can not run as superuser, credential were empty for target: %v", target.URL)
	}
	credentials[sudoCredentialKey] = target.Credential
	execution := &Execution{
		Credentials: credentials,
		MatchOutput: "Password",
		Command:     sudoCredentialKey,
		Error:       []string{"Password"},
		Extraction:  extractions,
	}
	execution.Error = append(execution.Error, errors...)
	result.ManagedCommand.Executions = append(result.ManagedCommand.Executions, execution)
	return result, nil
}
