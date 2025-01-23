package controllers

import (
	"context"
	"fmt"
	"reflect"

	"k8s.io/client-go/util/retry"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TryToUpdateStatus(ctx context.Context, client ctrlruntimeclient.Client, object ctrlruntimeclient.Object) error {
	key := ctrlruntimeclient.ObjectKeyFromObject(object)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		original := object.DeepCopyObject().(ctrlruntimeclient.Object)
		if err := client.Get(ctx, key, object); err != nil {
			return fmt.Errorf("could not fetch current %s/%s state, got error: %+v", object.GetName(), object.GetNamespace(), err)
		}

		if reflect.DeepEqual(object, original) {
			return nil
		}

		return client.Status().Patch(ctx, original, ctrlruntimeclient.MergeFrom(object))
	})

}
