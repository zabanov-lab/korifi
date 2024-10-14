package managed

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"code.cloudfoundry.org/korifi/controllers/controllers/services/credentials"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
)

type ManagedCredentialsReconciler struct {
	k8sClient           client.Client
	osbapiClientFactory osbapi.BrokerClientFactory
	scheme              *runtime.Scheme
	assets              *osbapi.Assets
}

func NewReconciler(k8sClient client.Client, brokerClientFactory osbapi.BrokerClientFactory, rootNamespace string, scheme *runtime.Scheme) *ManagedCredentialsReconciler {
	return &ManagedCredentialsReconciler{
		k8sClient:           k8sClient,
		osbapiClientFactory: brokerClientFactory,
		scheme:              scheme,
		assets:              osbapi.NewAssets(k8sClient, rootNamespace),
	}
}

func (r *ManagedCredentialsReconciler) ReconcileResource(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	cfServiceInstance := new(korifiv1alpha1.CFServiceInstance)
	err := r.k8sClient.Get(ctx, types.NamespacedName{Name: cfServiceBinding.Spec.Service.Name, Namespace: cfServiceBinding.Namespace}, cfServiceInstance)
	if err != nil {
		log.Info("service instance not found", "service-instance", cfServiceBinding.Spec.Service.Name, "error", err)
		return ctrl.Result{}, err
	}

	servicePlan, err := r.assets.GetServicePlan(ctx, cfServiceInstance.Spec.PlanGUID)
	if err != nil {
		log.Error(err, "failed to get service plan")
		return ctrl.Result{}, err
	}

	serviceBroker, err := r.assets.GetServiceBroker(ctx, servicePlan.Labels[korifiv1alpha1.RelServiceBrokerGUIDLabel])
	if err != nil {
		log.Error(err, "failed to get service broker")
		return ctrl.Result{}, err
	}

	serviceOffering, err := r.assets.GetServiceOffering(ctx, servicePlan.Labels[korifiv1alpha1.RelServiceOfferingGUIDLabel])
	if err != nil {
		log.Error(err, "failed to get service offering")
		return ctrl.Result{}, err
	}

	osbapiClient, err := r.osbapiClientFactory.CreateClient(ctx, serviceBroker)
	if err != nil {
		log.Error(err, "failed to create broker client", "broker", serviceBroker.Name)
		return ctrl.Result{}, fmt.Errorf("failed to create client for broker %q: %w", serviceBroker.Name, err)
	}

	var provisionResponse osbapi.BindResponse
	provisionResponse, err = osbapiClient.Bind(ctx, osbapi.BindPayload{
		BindingID:  cfServiceBinding.Name,
		InstanceID: cfServiceInstance.Name,
		BindRequest: osbapi.BindRequest{
			ServiceId: serviceOffering.Spec.BrokerCatalog.ID,
			PlanID:    servicePlan.Spec.BrokerCatalog.ID,
			BindResource: osbapi.BindResource{
				AppGUID: cfServiceBinding.Namespace + "/" + cfServiceBinding.Spec.AppRef.Name,
			},
			Parameters: map[string]any{},
		},
	})
	if err != nil {
		log.Error(err, "failed to bind service")
		return ctrl.Result{}, fmt.Errorf("failed to bind service: %w", err)
	}

	err = r.reconcileCredentials(ctx, cfServiceBinding, provisionResponse.Credentials)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *ManagedCredentialsReconciler) reconcileCredentials(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding, creds map[string]any) error {
	credentialsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfServiceBinding.Name,
			Namespace: cfServiceBinding.Namespace,
		},
	}
	_, err := controllerutil.CreateOrPatch(ctx, r.k8sClient, credentialsSecret, func() error {
		credentialsSecretData, err := tools.ToCredentialsSecretData(creds)
		if err != nil {
			return err
		}
		credentialsSecret.Data = credentialsSecretData
		return controllerutil.SetControllerReference(cfServiceBinding, credentialsSecret, r.scheme)
	})
	if err != nil {
		// TODO: fail the instance
		return fmt.Errorf("failed to create credentials secret: %w", err)
	}
	cfServiceBinding.Status.Credentials.Name = credentialsSecret.Name

	bindingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uuid.NewString(),
			Namespace: cfServiceBinding.Namespace,
		},
	}
	_, err = controllerutil.CreateOrPatch(ctx, r.k8sClient, bindingSecret, func() error {
		bindingSecret.Type = corev1.SecretType(credentials.ServiceBindingSecretTypePrefix + korifiv1alpha1.ManagedType)
		bindingSecret.Data, err = credentials.GetServiceBindingIOSecretData(credentialsSecret)
		if err != nil {
			return err
		}

		return controllerutil.SetControllerReference(cfServiceBinding, bindingSecret, r.scheme)
	})
	if err != nil {
		// fail the instance
		return errors.Wrap(err, "failed to create binding secret")
	}

	cfServiceBinding.Status.Binding.Name = bindingSecret.Name

	return nil
}
