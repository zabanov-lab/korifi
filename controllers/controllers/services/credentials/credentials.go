package credentials

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const ServiceBindingSecretTypePrefix = "servicebinding.io/"

type CredentialsReconciler struct {
	k8sClient client.Client
	log       logr.Logger
	scheme    *runtime.Scheme
}

func NewCredentialsReconciler(k8sClient client.Client, log logr.Logger, scheme *runtime.Scheme) *CredentialsReconciler {
	return &CredentialsReconciler{
		k8sClient: k8sClient,
		log:       log,
		scheme:    scheme,
	}
}

func (r *CredentialsReconciler) ReconcileResource(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)
	// start upsiCredentialsReconsiler.ReconcileResource

	cfServiceInstance := new(korifiv1alpha1.CFServiceInstance)
	err := r.k8sClient.Get(ctx, types.NamespacedName{Name: cfServiceBinding.Spec.Service.Name, Namespace: cfServiceBinding.Namespace}, cfServiceInstance)
	if err != nil {
		log.Info("service instance not found", "service-instance", cfServiceBinding.Spec.Service.Name, "error", err)
		return ctrl.Result{}, err
	}

	if cfServiceInstance.Status.Credentials.Name == "" {
		return ctrl.Result{}, k8s.NewNotReadyError().
			WithReason("CredentialsSecretNotAvailable").
			WithMessage("Service instance credentials not available yet").
			WithRequeueAfter(time.Second)
	}

	err = r.reconcileCredentials(ctx, cfServiceInstance, cfServiceBinding)
	if err != nil {
		if k8serrors.IsInvalid(err) {
			err = r.k8sClient.Delete(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfServiceBinding.Name,
					Namespace: cfServiceBinding.Namespace,
				},
			})
			return ctrl.Result{Requeue: true}, errors.Wrap(err, "failed to delete outdated binding secret")
		}

		log.Error(err, "failed to reconcile credentials secret")
		return ctrl.Result{}, err
	}

	// end of upsiCredentialsReconsiler.ReconcileResource

	return ctrl.Result{}, nil
}

func isLegacyServiceBinding(cfServiceBinding *korifiv1alpha1.CFServiceBinding, cfServiceInstance *korifiv1alpha1.CFServiceInstance) bool {
	if cfServiceBinding.Status.Binding.Name == "" {
		return false
	}

	// When reconciling existing legacy service bindings we make
	// use of the fact that the service binding used to reference
	// the secret of the sevice instance that shares the sevice
	// instance name. See ADR 16 for more datails.
	return cfServiceInstance.Name == cfServiceBinding.Status.Binding.Name && cfServiceInstance.Spec.SecretName == cfServiceBinding.Status.Binding.Name
}

func (r *CredentialsReconciler) reconcileCredentials(ctx context.Context, cfServiceInstance *korifiv1alpha1.CFServiceInstance, cfServiceBinding *korifiv1alpha1.CFServiceBinding) error {
	cfServiceBinding.Status.Credentials.Name = cfServiceInstance.Status.Credentials.Name

	if isLegacyServiceBinding(cfServiceBinding, cfServiceInstance) {
		bindingSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cfServiceBinding.Status.Binding.Name,
				Namespace: cfServiceBinding.Namespace,
			},
		}

		// For legacy sevice bindings we want to keep the binding secret
		// unchanged in order to avoid unexpected app restarts. See ADR 16 for more details.
		err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(bindingSecret), bindingSecret)
		if err != nil {
			return err
		}

		return nil
	}

	credentialsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfServiceInstance.Namespace,
			Name:      cfServiceInstance.Status.Credentials.Name,
		},
	}
	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(credentialsSecret), credentialsSecret)
	if err != nil {
		return fmt.Errorf("failed to get service instance credentials secret %q: %w", cfServiceInstance.Status.Credentials.Name, err)
	}

	bindingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfServiceBinding.Name,
			Namespace: cfServiceBinding.Namespace,
		},
	}

	_, err = controllerutil.CreateOrPatch(ctx, r.k8sClient, bindingSecret, func() error {
		bindingSecret.Type, err = GetBindingSecretType(credentialsSecret)
		if err != nil {
			return err
		}
		bindingSecret.Data, err = GetServiceBindingIOSecretData(credentialsSecret)
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

func GetBindingSecretType(credentialsSecret *corev1.Secret) (corev1.SecretType, error) {
	credentials := map[string]any{}
	err := GetCredentials(credentialsSecret, &credentials)
	if err != nil {
		return "", err
	}

	userProvidedType, isString := credentials["type"].(string)
	if isString {
		return corev1.SecretType(ServiceBindingSecretTypePrefix + userProvidedType), nil
	}

	return corev1.SecretType(ServiceBindingSecretTypePrefix + korifiv1alpha1.UserProvidedType), nil
}

func GetServiceBindingIOSecretData(credentialsSecret *corev1.Secret) (map[string][]byte, error) {
	credentials := map[string]any{}
	err := GetCredentials(credentialsSecret, &credentials)
	if err != nil {
		return nil, err
	}
	secretData := map[string][]byte{}
	for k, v := range credentials {
		secretData[k], err = toBytes(v)
		if err != nil {
			return nil, fmt.Errorf("failed to convert value of key %q to bytes: %w", k, err)
		}
	}

	if _, hasType := secretData["type"]; !hasType {
		secretData["type"] = []byte("user-provided")
	}

	return secretData, err
}

func toBytes(value any) ([]byte, error) {
	valueString, ok := value.(string)
	if ok {
		return []byte(valueString), nil
	}

	return json.Marshal(value)
}

func GetCredentials(credentialsSecret *corev1.Secret, credentialsObject any) error {
	credentials, ok := credentialsSecret.Data[tools.CredentialsSecretKey]
	if !ok {
		return fmt.Errorf(
			"data of secret %q does not contain the %q key",
			credentialsSecret.Name,
			tools.CredentialsSecretKey,
		)
	}

	return errors.Wrap(json.Unmarshal(credentials, credentialsObject), "failed to unmarshal secret data")
}
