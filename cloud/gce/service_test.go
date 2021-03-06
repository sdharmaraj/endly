package gce_test

import (
	"github.com/stretchr/testify/assert"
	"github.com/viant/endly"
	"github.com/viant/endly/cloud/gce"
	"github.com/viant/toolbox"
	"github.com/viant/toolbox/secret"
	"google.golang.org/api/compute/v1"
	"os"
	"path"
	"testing"
)

//https://cloud.google.com/compute/docs/reference/beta/instances/start
var secrets = secret.New("", false)

func TestNewGceService(t *testing.T) {

	credentials := path.Join(os.Getenv("HOME"), ".secret/gce.json")
	if toolbox.FileExists(credentials) && os.Getenv("GCE_PROJECT") != "" {
		manager := endly.New()
		context := manager.NewContext(toolbox.NewContext())
		service, _ := context.Service(gce.ServiceID)

		project := os.Getenv("GCE_PROJECT")
		zone := "us-west1-b"
		instance := "instance-1"
		serviceResponse := service.Run(context, &gce.CallRequest{
			Credentials: credentials,
			Service:     "Instances",
			Method:      "Get",
			Parameters:  []interface{}{project, zone, instance},
		})
		assert.Equal(t, "", serviceResponse.Error)
		if gceResponse, ok := serviceResponse.Response.(gce.CallResponse); ok && gceResponse != nil {
			if instance, ok := gceResponse.(*compute.Instance); ok {
				assert.EqualValues(t, "instance-1", instance.Name)
			}
		}
	}
}

func TestNewGceService_WithError(t *testing.T) {
	parent := toolbox.CallerDirectory(3)
	credentials := path.Join(parent, "test/gce/secret.json")
	manager := endly.New()
	context := manager.NewContext(toolbox.NewContext())
	service, _ := context.Service(gce.ServiceID)

	{ //test non existing method
		project := os.Getenv("GCE_PROJECT")
		zone := "us-west1-b "
		instance := "instance-1 "
		serviceResponse := service.Run(context, &gce.CallRequest{
			Credentials: credentials,
			Service:     "Instances",
			Method:      "Get1",
			Parameters:  []interface{}{project, zone, instance},
		})
		assert.True(t, serviceResponse.Error != "")
	}

	{ //test auth isseue
		project := "dummy"
		zone := "us-west1-b"
		instance := "instance-1"
		serviceResponse := service.Run(context, &gce.CallRequest{
			Credentials: credentials,
			Service:     "Instances",
			Method:      "Get",
			Parameters:  []interface{}{project, zone, instance},
		})
		assert.True(t, serviceResponse.Error != "")

	}
	{ //test auth isseue
		project := "dummy"
		zone := "us-west1-b"
		instance := "instance-1"
		serviceResponse := service.Run(context, &gce.CallRequest{
			Credentials: credentials,
			Service:     "Instances",
			Method:      "List",
			Parameters:  []interface{}{project, zone, instance},
		})
		assert.True(t, serviceResponse.Error != "")
	}

	{ //missing credentials
		project := "dummy"
		zone := "us-west1-b"
		instance := "instance-1"
		serviceResponse := service.Run(context, &gce.CallRequest{
			Credentials: credentials,
			Service:     "Instances",
			Method:      "List",
			Parameters:  []interface{}{project, zone, instance},
		})
		assert.True(t, serviceResponse.Error != "")
	}

}

func TestGCEService_NewRequest(t *testing.T) {
	parent := toolbox.CallerDirectory(3)
	credentials := path.Join(parent, "test/secret.json")
	credConfig, err := secrets.GetCredentials(credentials)
	assert.Nil(t, err)
	service, ctx, err := gce.NewComputeService(credConfig)
	assert.Nil(t, err)
	assert.NotNil(t, service)
	assert.NotNil(t, ctx)

}

func Test_NewComputeService(t *testing.T) {
	_, _, err := gce.NewComputeService(nil)
	assert.NotNil(t, err)

}
