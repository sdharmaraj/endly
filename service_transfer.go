package endly

import (
	"fmt"
	"github.com/viant/toolbox"
	"github.com/viant/toolbox/storage"
	"io"
	"io/ioutil"
	"strings"
)

//TransferServiceID represents transfer service id
const TransferServiceID = "transfer"

type transferService struct {
	*AbstractService
}

//NewExpandedContentHandler return a new reader that can substitude content with state map, replacement data provided in replacement map.
func NewExpandedContentHandler(context *Context, replaceMap map[string]string, expand bool) func(reader io.Reader) (io.Reader, error) {
	return func(reader io.Reader) (io.Reader, error) {
		content, err := ioutil.ReadAll(reader)
		if err != nil {
			return nil, err
		}
		var result = string(content)
		if expand {
			result = context.Expand(result)
			if err != nil {
				return nil, err
			}
		}
		for k, v := range replaceMap {
			result = strings.Replace(result, k, v, len(result))
		}
		return strings.NewReader(toolbox.AsString(result)), nil
	}
}

func (s *transferService) run(context *Context, transfers ...*Transfer) (*TransferCopyResponse, error) {
	var result = &TransferCopyResponse{
		Transferred: make([]*TransferLog, 0),
	}
	for _, transfer := range transfers {
		source, err := context.ExpandResource(transfer.Source)
		if err != nil {
			return nil, err
		}
		sourceService, err := storage.NewServiceForURL(source.URL, source.Credential)
		if err != nil {
			return nil, err
		}
		target, err := context.ExpandResource(transfer.Target)
		if err != nil {
			return nil, err
		}
		targetService, err := storage.NewServiceForURL(target.URL, target.Credential)
		if err != nil {
			return nil, fmt.Errorf("Failed to lookup target storageService for %v: %v", target.URL, err)
		}
		var handler func(reader io.Reader) (io.Reader, error)
		if transfer.Expand || len(transfer.Replace) > 0 {
			handler = NewExpandedContentHandler(context, transfer.Replace, transfer.Expand)
		}
		if _, err := sourceService.StorageObject(source.URL); err != nil {
			return nil, fmt.Errorf("Failed to copy: %v %v - Source does not exists", source.URL, target.URL)
		}
		startEvent := s.Begin(context, transfer, Pairs("source", source.URL, "target", target.URL, "expand", transfer.Expand || len(transfer.Replace) > 0), Info)
		err = storage.Copy(sourceService, source.URL, targetService, target.URL, handler)
		s.End(context)(startEvent, Pairs())
		info := NewTransferLog(context, source.URL, target.URL, err, transfer.Expand)
		result.Transferred = append(result.Transferred, info)
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func (s *transferService) Run(context *Context, request interface{}) *ServiceResponse {
	startEvent := s.Begin(context, request, Pairs("request", request))
	var response = &ServiceResponse{Status: "ok"}
	defer s.End(context)(startEvent, Pairs("response", response))
	var err error
	switch actualRequest := request.(type) {
	case *TransferCopyRequest:
		response.Response, err = s.run(context, actualRequest.Transfers...)
		if err != nil {
			response.Error = fmt.Sprintf("Failed to tranfer resources: %v, %v", actualRequest.Transfers, err)
		}
	default:
		response.Error = fmt.Sprintf("Unsupported request type: %T", request)
	}
	if response.Error != "" {
		response.Status = "err"
	}
	return response
}

func (s *transferService) NewRequest(action string) (interface{}, error) {
	switch action {
	case "copy":
		return &TransferCopyRequest{
			Transfers: make([]*Transfer, 0),
		}, nil
	}
	return s.AbstractService.NewRequest(action)
}

//NewTransferService creates a new transfer service
func NewTransferService() Service {
	var result = &transferService{
		AbstractService: NewAbstractService(TransferServiceID),
	}
	result.AbstractService.Service = result
	return result

}
