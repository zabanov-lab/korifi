package managed

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi"
	"github.com/go-logr/logr"
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
}

func NewReconciler(k8sClient client.Client, brokerClientFactory osbapi.BrokerClientFactory, scheme *runtime.Scheme) *ManagedCredentialsReconciler {
	return &ManagedCredentialsReconciler{
		k8sClient:           k8sClient,
		osbapiClientFactory: brokerClientFactory,
		scheme:              scheme,
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

	// if cfServiceInstance.Status.Credentials.Name == "" {
	// 	return ctrl.Result{}, k8s.NewNotReadyError().
	// 		WithReason("CredentialsSecretNotAvailable").
	// 		WithMessage("Service instance credentials not available yet").
	// 		WithRequeueAfter(time.Second)
	// }

	servicePlan, err := r.getServicePlan(ctx, cfServiceInstance.Spec.PlanGUID, cfServiceBinding.Namespace)
	if err != nil {
		log.Error(err, "failed to get service plan")
		return ctrl.Result{}, err
	}

	serviceBroker, err := r.getServiceBroker(ctx, servicePlan.Labels[korifiv1alpha1.RelServiceBrokerGUIDLabel], cfServiceBinding.Namespace)
	if err != nil {
		log.Error(err, "failed to get service broker")
		return ctrl.Result{}, err
	}

	serviceOffering, err := r.getServiceOffering(ctx, servicePlan.Labels[korifiv1alpha1.RelServiceOfferingGUIDLabel], cfServiceBinding.Namespace)
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
			AppGUID:   cfServiceBinding.Spec.AppRef.Name,
			BindResource: osbapi.BindResource{
				AppGUID: cfServiceBinding.Spec.AppRef.Name,
			},
			Parameters: map[string]any{},
		},
	})
	if err != nil {
		log.Error(err, "failed to bind service")
		return ctrl.Result{}, fmt.Errorf("failed to bind service: %w", err)
	}

	err = r.reconcileCredentials(ctx, cfServiceInstance, cfServiceBinding, provisionResponse.Credentials)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *ManagedCredentialsReconciler) reconcileCredentials(ctx context.Context, cfServiceInstance *korifiv1alpha1.CFServiceInstance, cfServiceBinding *korifiv1alpha1.CFServiceBinding, creds map[string]any) error {
	cfServiceBinding.Status.Credentials.Name = cfServiceInstance.Status.Credentials.Name

	bindingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfServiceBinding.Name,
			Namespace: cfServiceBinding.Namespace,
		},
	}
	var err error
	_, err = controllerutil.CreateOrPatch(ctx, r.k8sClient, bindingSecret, func() error {
		bindingSecret.Type = corev1.SecretType(credentials.ServiceBindingSecretTypePrefix + korifiv1alpha1.ManagedType)
		bindingSecret.Data, err = r.getSecretData(creds)
		if err != nil {
			return err
		}

		return controllerutil.SetControllerReference(cfServiceBinding, bindingSecret, r.scheme)
	})
	if err != nil {
		return errors.Wrap(err, "failed to create binding secret")
	}

	cfServiceBinding.Status.Binding.Name = bindingSecret.Name

	return nil
}

func (r *ManagedCredentialsReconciler) getSecretData(data map[string]any) (map[string][]byte, error) {
	secretData := map[string][]byte{}
	var err error
	for k, v := range data {
		secretData[k], err = credentials.ToBytes(v)
		if err != nil {
			return nil, err
		}
	}

	if _, hasType := secretData["type"]; !hasType {
		secretData["type"] = []byte("managed")
	}
	return secretData, nil
}

func (r *ManagedCredentialsReconciler) getServiceOffering(ctx context.Context, offeringGUID string, namespace string) (*korifiv1alpha1.CFServiceOffering, error) {
	serviceOffering := &korifiv1alpha1.CFServiceOffering{
		ObjectMeta: metav1.ObjectMeta{
			Name:      offeringGUID,
			Namespace: namespace,
		},
	}
	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceOffering), serviceOffering)
	if err != nil {
		return nil, fmt.Errorf("failed to get service offering %q: %w", offeringGUID, err)
	}

	return serviceOffering, nil
}

func (r *ManagedCredentialsReconciler) getServicePlan(ctx context.Context, planGUID string, namespace string) (*korifiv1alpha1.CFServicePlan, error) {
	servicePlan := &korifiv1alpha1.CFServicePlan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      planGUID,
			Namespace: namespace,
		},
	}
	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(servicePlan), servicePlan)
	if err != nil {
		return nil, fmt.Errorf("failed to get service plan %q: %w", planGUID, err)
	}
	return servicePlan, nil
}

func (r *ManagedCredentialsReconciler) getServiceBroker(ctx context.Context, brokerGUID string, namespace string) (*korifiv1alpha1.CFServiceBroker, error) {
	serviceBroker := &korifiv1alpha1.CFServiceBroker{
		ObjectMeta: metav1.ObjectMeta{
			Name:      brokerGUID,
			Namespace: namespace,
		},
	}
	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceBroker), serviceBroker)
	if err != nil {
		return nil, fmt.Errorf("failed to get service broker %q: %w", brokerGUID, err)
	}

	return serviceBroker, nil
}
