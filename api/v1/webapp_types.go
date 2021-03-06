/*
Copyright 2022 mobfun.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	core1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// WebAppSpec defines the desired state of WebApp
type WebAppSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Replicas      *int32                `json:"replicas"`
	Image         string                `json:"image"`
	Command       []string              `json:"command,omitempty"`
	Args          []string              `json:"args,omitempty"`
	Env           []core1.EnvVar        `json:"env,omitempty"`
	HostPathMount []HostPathMountConfig `json:"hostPathMount,omitempty"`
	HostBinding   []string              `json:"hostBinding,omitempty"`
	HostName      string                `json:"hostName"`
	ContextPath   string                `json:"contextPath"`
	Promtail      PromtailConfig        `json:"promTail,omitempty"`
}

type PromtailConfig struct {
	Image       string `json:"image,omitempty"`
	PromtailYml string `json:"promtailYml"`
}

func (s PromtailConfig) IsEmpty() bool {
	return reflect.DeepEqual(s, PromtailConfig{})
}

type HostPathMountConfig struct {
	//挂载简洁
	DescribePath string `json:"describePath"`
	//容器路径
	Spath string `json:"spath,omitempty"`
	//服务器路径
	Dpath string `json:"dpath,omitempty"`
}

// WebAppStatus defines the observed state of WebApp
type WebAppStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// WebApp is the Schema for the webapps API
type WebApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WebAppSpec   `json:"spec,omitempty"`
	Status WebAppStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// WebAppList contains a list of WebApp
type WebAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WebApp `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WebApp{}, &WebAppList{})
}
