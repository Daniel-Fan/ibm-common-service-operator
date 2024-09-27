//
// Copyright 2022 IBM Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package operandrequest

import (
	"context"
	"net/http"

	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/mutate-operator-ibm-com-v1alpha1-keycloak,mutating=true,failurePolicy=fail,sideEffects=None,groups=k8s.keycloak.org,resources=keycloaks,verbs=create;update,versions=v2alpha1,name=mkeycloak.kb.io,admissionReviewVersions=v1

// KeycloakDefaulter configure correct resource configuration
type Defaulter struct {
	Reader    client.Reader
	Client    client.Client
	IsDormant bool
	decoder   *admission.Decoder
}

// KeycloakDefaulter mutates Keycloak CR based on the version of Keycloak CRD installed
func (r *Defaulter) Handle(ctx context.Context, req admission.Request) admission.Response {
	klog.Infof("Webhook is invoked by Keycloak %s/%s", req.AdmissionRequest.Namespace, req.AdmissionRequest.Name)

	if r.IsDormant {
		return admission.Allowed("")
	}

	// Get the CRD of Keycloak and check if .spec.resources is available
	keycloakCRD := &apiextensions.CustomResourceDefinition{}
	err := r.Reader.Get(ctx, types.NamespacedName{Name: "keycloaks.k8s.keycloak.org"}, keycloakCRD)
	if err != nil {
		klog.Errorf("Failed to get CRD keycloaks.k8s.keycloak.org: %v", err)
		return admission.Errored(http.StatusInternalServerError, err)
	}

	// Check if .spec.resources is available in the CRD
	if keycloakCRD.Spec.Versions[0].Schema.OpenAPIV3Schema != nil {
		if specProperty, exists := keycloakCRD.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"]; exists {
			// Decode the Keycloak CR
			keycloak := &unstructured.Unstructured{}
			keycloak.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   req.Kind.Group,
				Version: req.Kind.Version,
				Kind:    req.Kind.Kind,
			})
			if err := r.decoder.Decode(req, keycloak); err != nil {
				klog.Errorf("Failed to decode Keycloak CR: %v", err)
				return admission.Errored(http.StatusBadRequest, err)
			}
			if _, exists := specProperty.Properties["resources"]; !exists {
				klog.Infof("CRD keycloaks.k8s.keycloak.org does not have .spec.resources, mutating the Keycloak CR")

				// Get the ".spec.resources" property of the Keycloak CR
				resources, _, err := unstructured.NestedMap(keycloak.Object, "spec", "resources")
				if err != nil {
					klog.Errorf("Failed to get .spec.resources of Keycloak CR: %v", err)
					return admission.Errored(http.StatusInternalServerError, err)
				}

				// Convert .spec.resources to .spec.unsupported.podTemplate.spec.containers[0].resources
				// If the field does not exist, create it
				if resources != nil {
					if _, exists := keycloak.Object["spec"]; !exists {
						return admission.Errored(http.StatusInternalServerError, errors.NewBadRequest("Keycloak CR does not have .spec field"))
					}
					if _, exists := keycloak.Object["spec"].(map[string]interface{})["unsupported"]; !exists {
						keycloak.Object["spec"].(map[string]interface{})["unsupported"] = map[string]interface{}{}
					}
					if _, exists := keycloak.Object["spec"].(map[string]interface{})["unsupported"].(map[string]interface{})["podTemplate"]; !exists {
						keycloak.Object["spec"].(map[string]interface{})["unsupported"].(map[string]interface{})["podTemplate"] = map[string]interface{}{}
					}
					if _, exists := keycloak.Object["spec"].(map[string]interface{})["unsupported"].(map[string]interface{})["podTemplate"].(map[string]interface{})["spec"]; !exists {
						keycloak.Object["spec"].(map[string]interface{})["unsupported"].(map[string]interface{})["podTemplate"].(map[string]interface{})["spec"] = map[string]interface{}{}
					}
					if _, exists := keycloak.Object["spec"].(map[string]interface{})["unsupported"].(map[string]interface{})["podTemplate"].(map[string]interface{})["spec"].(map[string]interface{})["containers"]; !exists {
						keycloak.Object["spec"].(map[string]interface{})["unsupported"].(map[string]interface{})["podTemplate"].(map[string]interface{})["spec"].(map[string]interface{})["containers"] = []map[string]interface{}{}
					}
					if container := keycloak.Object["spec"].(map[string]interface{})["unsupported"].(map[string]interface{})["podTemplate"].(map[string]interface{})["spec"].(map[string]interface{})["containers"].([]map[string]interface{})[0]; container == nil {
						keycloak.Object["spec"].(map[string]interface{})["unsupported"].(map[string]interface{})["podTemplate"].(map[string]interface{})["spec"].(map[string]interface{})["containers"].([]map[string]interface{})[0] = map[string]interface{}{}
					}
					keycloak.Object["spec"].(map[string]interface{})["unsupported"].(map[string]interface{})["podTemplate"].(map[string]interface{})["spec"].(map[string]interface{})["containers"].([]map[string]interface{})[0]["resources"] = resources
				}
				// Remove the ".spec.resources" property
				if err := unstructured.SetNestedField(keycloak.Object, nil, "spec", "resources"); err != nil {
					klog.Errorf("Failed to remove .spec.resources of Keycloak CR: %v", err)
					return admission.Errored(http.StatusInternalServerError, err)
				}
			} else {
				// Remove .spec.unsupported.podTemplate.spec.containers[0].resources if .spec.resources is available
				if _, exists := keycloak.Object["spec"]; exists {
					if _, exists := keycloak.Object["spec"].(map[string]interface{})["unsupported"]; exists {
						if _, exists := keycloak.Object["spec"].(map[string]interface{})["unsupported"].(map[string]interface{})["podTemplate"]; exists {
							if _, exists := keycloak.Object["spec"].(map[string]interface{})["unsupported"].(map[string]interface{})["podTemplate"].(map[string]interface{})["spec"]; exists {
								if _, exists := keycloak.Object["spec"].(map[string]interface{})["unsupported"].(map[string]interface{})["podTemplate"].(map[string]interface{})["spec"].(map[string]interface{})["containers"]; exists {
									if container := keycloak.Object["spec"].(map[string]interface{})["unsupported"].(map[string]interface{})["podTemplate"].(map[string]interface{})["spec"].(map[string]interface{})["containers"].([]map[string]interface{})[0]; container != nil {
										delete(container, "resources")
									}
								}
							}
						}
					}
				}
			}
			// Marshal the mutated Keycloak CR
			marshaledKeycloak, err := keycloak.MarshalJSON()
			if err != nil {
				klog.Errorf("Failed to marshal mutated Keycloak CR: %v", err)
				return admission.Errored(http.StatusInternalServerError, err)
			}

			// Return the mutated Keycloak CR
			return admission.PatchResponseFromRaw(req.Object.Raw, marshaledKeycloak)
		}
	}

	return admission.Errored(http.StatusInternalServerError, errors.NewBadRequest("Failed to interpret CRD keycloaks.k8s.keycloak.org"))
}

func (r *Defaulter) InjectDecoder(decoder *admission.Decoder) error {
	r.decoder = decoder
	return nil
}

func (r *Defaulter) SetupWebhookWithManager(mgr ctrl.Manager) error {

	mgr.GetWebhookServer().
		Register("/mutate-operator-ibm-com-v1alpha1-keycloak",
			&webhook.Admission{Handler: r})

	return nil
}
