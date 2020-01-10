/*

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

package controllers

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"

	batchv1 "example/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	errors "k8s.io/apimachinery/pkg/api/errors"
	//types "k8s.io/apimachinery/pkg/types"
	//"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// GenericDaemonReconciler reconciles a GenericDaemon object
type GenericDaemonReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=batch.tutorial.kubebuilder.io,resources=genericdaemons,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch.tutorial.kubebuilder.io,resources=genericdaemons/status,verbs=get;update;patch

func (r *GenericDaemonReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("GenericDaemon", req.NamespacedName)
	//var gd batchv1.GenericDaemon
	gd := &batchv1.GenericDaemon{}
	err := r.Get(ctx, req.NamespacedName, gd)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Generic daemon not found. Deleted ! \n")
		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	//Create a service first
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gd.Name,
			Namespace: gd.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				corev1.ServicePort{
					Protocol: corev1.Protocol(gd.Spec.Protocol),
					Port:     gd.Spec.ServicePort,
					//TargetPort: 80,
				},
			},
			Selector: map[string]string{"statefulset": gd.Name + "-statefulset"},
			Type:     corev1.ServiceType(gd.Spec.ServiceType),
		},
	}
	err = r.Get(ctx, req.NamespacedName, service)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating Service \n", service.Namespace, service.Name)
		err := r.Create(ctx, service)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else if err != nil {
		return ctrl.Result{}, err
	}
	// your logic here
	var replica int32 = gd.Spec.Replica
	fmt.Println("gd.Name:", gd.Name, "gd.Status.Count:", gd.Status.Count)
	// Define the desired statefulset object
	statefulset := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gd.Name + "-statefulset",
			Namespace: gd.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replica,
			ServiceName: gd.Name,
			//MinReadySeconds: 5,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"statefulset": gd.Name + "-statefulset"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"statefulset": gd.Name + "-statefulset"},
				},
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{"daemon": gd.Spec.Label},
					Containers: []corev1.Container{
						{
							Name:  "genericdaemon",
							Image: gd.Spec.Image,
						},
					},
				},
			},
		},
	}
	if err := ctrl.SetControllerReference(gd, statefulset, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	found := &appsv1.StatefulSet{}
	//err := r.Get(ctx, types.NamespacedName{Name: statefulset.Name, Namespace: statefulset.Namespace}, found)
	err = r.Get(ctx, client.ObjectKey{Namespace: statefulset.Namespace, Name: statefulset.Name}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating statefulset \n", statefulset.Namespace, statefulset.Name)
		fmt.Println("statefulset.Status.Replicas:", statefulset.Status.Replicas)
		err := r.Create(ctx, statefulset)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else if err != nil {
		return ctrl.Result{}, err
	}
	//Wait for the statefulset to come up
	time.Sleep(15 * time.Second)
	err = r.Get(ctx, client.ObjectKey{Namespace: statefulset.Namespace, Name: statefulset.Name}, found)
	// Get the number of Ready statefulsets and set the Count status
	if err == nil && found.Status.CurrentReplicas != gd.Status.Count {
		log.Info("Updating Status \n", gd.Namespace, gd.Name)
		gd.Status.Count = found.Status.CurrentReplicas
		err = r.Update(ctx, gd)
		if err != nil {
			return ctrl.Result{}, err
		}
	}
	err = r.Get(ctx, client.ObjectKey{Namespace: statefulset.Namespace, Name: statefulset.Name}, found)
	fmt.Println("found.Name:", found.Name)
	// Update the found object and write the result back if there are any changes
	if !reflect.DeepEqual(statefulset.Spec, found.Spec) {
		found.Spec = statefulset.Spec
		log.Info("Updating statefulset ", statefulset.Namespace, statefulset.Name)
		fmt.Println("found.Name:", found.Name)
		//fmt.Println("found.Spec.Label:", found.Spec.Template.Spec.Containers.)
		err = r.Update(ctx, found)
		if err != nil {
			return ctrl.Result{}, err
		}
	}
	//Install the packages in pod
	pod := corev1.Pod{}
	podList := &corev1.PodList{}

	err = r.List(ctx, podList, client.InNamespace(req.Namespace), client.MatchingLabels{"statefulset": gd.Name + "-statefulset"})
	numberPods := len((*podList).Items)
	fmt.Println("numberPods:", numberPods)
	for i := 0; i < numberPods; i++ {
		pod = (*podList).Items[i]
		fmt.Println("PodName:", pod.ObjectMeta.Name)
	}
	//RequeueAfter: 60000000
	return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
}

func (r *GenericDaemonReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&batchv1.GenericDaemon{}).
		Complete(r)
}
