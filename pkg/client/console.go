package client

import (
	"context"
	"encoding/json"
	"net/http"

	rawclient "github.com/Yamashou/gqlgenc/clientv2"
	internalerror "github.com/kairos-io/osbuilder/pkg/errors"
	"github.com/kairos-io/osbuilder/pkg/helpers"
	"github.com/pkg/errors"
	console "github.com/pluralsh/console/go/client"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type client struct {
	ctx           context.Context
	consoleClient console.ConsoleClient
	url           string
	token         string
}

func New(url, token string) Client {
	return &client{
		consoleClient: console.NewClient(&http.Client{
			Transport: helpers.NewAuthorizationTokenTransport(token),
		}, url, nil),
		ctx:   context.Background(),
		url:   url,
		token: token,
	}
}

type Client interface {
	CreateClusterIsoImage(attributes console.ClusterIsoImageAttributes) (*console.ClusterIsoImageFragment, error)
	UpdateClusterIsoImage(id string, attributes console.ClusterIsoImageAttributes) (*console.ClusterIsoImageFragment, error)
	GetClusterIsoImage(image *string) (*console.ClusterIsoImageFragment, error)
	DeleteClusterIsoImage(id string) (*console.ClusterIsoImageFragment, error)
}

func (c *client) CreateClusterIsoImage(attributes console.ClusterIsoImageAttributes) (*console.ClusterIsoImageFragment, error) {
	response, err := c.consoleClient.CreateClusterIsoImage(c.ctx, attributes)
	if err != nil {
		return nil, err
	}

	return response.CreateClusterIsoImage, nil
}

func (c *client) DeleteClusterIsoImage(id string) (*console.ClusterIsoImageFragment, error) {
	response, err := c.consoleClient.DeleteClusterIsoImage(c.ctx, id)
	if err != nil {
		return nil, err
	}

	return response.DeleteClusterIsoImage, nil
}

func (c *client) UpdateClusterIsoImage(id string, attributes console.ClusterIsoImageAttributes) (*console.ClusterIsoImageFragment, error) {
	response, err := c.consoleClient.UpdateClusterIsoImage(c.ctx, id, attributes)
	if err != nil {
		return nil, err
	}

	return response.UpdateClusterIsoImage, nil
}

func (c *client) GetClusterIsoImage(image *string) (*console.ClusterIsoImageFragment, error) {
	response, err := c.consoleClient.GetClusterIsoImage(c.ctx, nil, image)
	if internalerror.IsNotFound(err) {
		return nil, apierrors.NewNotFound(schema.GroupResource{}, *image)
	}
	if err == nil && (response == nil || response.ClusterIsoImage == nil) {
		return nil, apierrors.NewNotFound(schema.GroupResource{}, *image)
	}

	if response == nil {
		return nil, err
	}

	return response.ClusterIsoImage, nil
}

func GetErrorResponse(err error, methodName string) error {
	if err == nil {
		return nil
	}
	errResponse := &rawclient.ErrorResponse{}
	newErr := json.Unmarshal([]byte(err.Error()), errResponse)
	if newErr != nil {
		return err
	}

	errList := errors.New(methodName)
	if errResponse.GqlErrors != nil {
		for _, err := range *errResponse.GqlErrors {
			errList = errors.Wrap(errList, err.Message)
		}
		errList = errors.Wrap(errList, "GraphQL error")
	}
	if errResponse.NetworkError != nil {
		errList = errors.Wrap(errList, errResponse.NetworkError.Message)
		errList = errors.Wrap(errList, "Network error")
	}

	return errList
}
