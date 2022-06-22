package controllers

import (
	"context"
	"fmt"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	ing "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	nas "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/json"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"strconv"
	"strings"
	mobfunv1 "vm-operator/api/v1"
)

//定义常量
const (
	Port                     = 8080
	oldDeploySpecAnnotation  = "old/deploySpec"
	oldIngressSpecAnnotation = "old/ingressSpec"
)

//修改的结构体
/*
创建旧的deployment状态
deployment支持修改副本数，镜像，挂载目录，绑定的主机
*/
type oldDeploy struct {
	Replicas      *int32                         `json:"replicas"`
	Image         string                         `json:"image"`
	HostPathMount []mobfunv1.HostPathMountConfig `json:"hostPathMount,omitempty"`
	HostBinding   []string                       `json:"hostBinding,omitempty"`
}

/*
创建旧的ingress状态
ingress支持修改主机名和拦截的uri
*/
type oldIngress struct {
	HostName    string `json:"hostName"`
	ContextPath string `json:"contextPath"`
}

//方法区

//更新Annotation
func updataSpecAnnotation(webApp *mobfunv1.WebApp, ctx context.Context, r *WebAppReconciler) error {
	olddep := oldDeploy{
		Replicas:      webApp.Spec.Replicas,
		Image:         webApp.Spec.Image,
		HostPathMount: webApp.Spec.HostPathMount,
		HostBinding:   webApp.Spec.HostBinding,
	}

	olding := oldIngress{
		HostName:    webApp.Spec.HostName,
		ContextPath: webApp.Spec.ContextPath,
	}
	depData, _ := json.Marshal(olddep)
	ingData, _ := json.Marshal(olding)
	if webApp.Annotations != nil {
		webApp.Annotations[oldDeploySpecAnnotation] = string(depData)
		webApp.Annotations[oldIngressSpecAnnotation] = string(ingData)
	} else {
		webApp.Annotations = map[string]string{
			oldDeploySpecAnnotation:  string(depData),
			oldIngressSpecAnnotation: string(ingData),
		}
	}
	if err := r.Client.Update(ctx, webApp); err != nil {
		return err
	}
	return nil
}

// 新建deployment
func createDeployment(ctx context.Context, r *WebAppReconciler, webApp *mobfunv1.WebApp, req ctrl.Request) error {
	k8slog := log.FromContext(ctx)
	deployment := &appsv1.Deployment{}

	err := r.Get(ctx, req.NamespacedName, deployment)

	//成功获取到deployment，走更新逻辑
	if err == nil {
		k8slog.Info("deployment exists")
		olddep := oldDeploy{}

		//获取老的Annotation
		if err := json.Unmarshal([]byte(webApp.Annotations[oldDeploySpecAnnotation]), &olddep); err != nil {
			return err
		}

		//构建当前的Annotation
		newdep := oldDeploy{
			Replicas:      webApp.Spec.Replicas,
			Image:         webApp.Spec.Image,
			HostPathMount: webApp.Spec.HostPathMount,
			HostBinding:   webApp.Spec.HostBinding,
		}

		//如果当前的Annotation状态和老的Annotation状态不一样则需要更新
		if !reflect.DeepEqual(newdep, olddep) {
			olddeploy := &appsv1.Deployment{}

			//获取到老的Deployment实例
			if err := r.Get(ctx, req.NamespacedName, olddeploy); err != nil {
				return err
			}

			//更新镜像和副本数
			olddeploy.Spec.Template.Spec.Containers[0].Image = newdep.Image
			olddeploy.Spec.Replicas = newdep.Replicas

			//如果有挂载就更新挂载
			if newdep.HostPathMount != nil {
				vlmms := []corev1.VolumeMount{}
				vlss := []corev1.Volume{}
				for i := range webApp.Spec.HostPathMount {
					fmt.Println(webApp.Spec.HostPathMount[i].Spath)
					fmt.Println(webApp.Spec.HostPathMount[i].Dpath)

					//创建Volume对象
					vlm := corev1.Volume{}
					vlm.HostPath = &corev1.HostPathVolumeSource{
						Path: webApp.Spec.HostPathMount[i].Dpath,
					}
					vlm.Name = "volume" + strconv.Itoa(i)
					vlss = append(vlss, vlm)

					//创建VolumeMount对象
					vlmm := corev1.VolumeMount{}
					vlmm.Name = "volume" + strconv.Itoa(i)
					vlmm.MountPath = webApp.Spec.HostPathMount[i].Spath
					vlmms = append(vlmms, vlmm)

				}
				olddeploy.Spec.Template.Spec.Volumes = vlss
				olddeploy.Spec.Template.Spec.Containers[0].VolumeMounts = vlmms
			}

			//如果有主机绑定就更新主机绑定
			if newdep.HostBinding != nil {
				aff := corev1.Affinity{
					NodeAffinity: &corev1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
							NodeSelectorTerms: []corev1.NodeSelectorTerm{
								{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "kubernetes.io/hostname",
											Operator: corev1.NodeSelectorOpIn,
											Values:   webApp.Spec.HostBinding,
										},
									},
								},
							},
						},
					},
				}
				olddeploy.Spec.Template.Spec.Affinity = &aff
			}

			if err := r.Client.Update(ctx, olddeploy); err != nil {
				return err
			}
		}
		//返回之后方法结束，就不用考虑下面的逻辑了
		return nil
	}

	// 如果错误不是NotFound，证明获取crd对象异常返回错误
	if !errors.IsNotFound(err) {
		k8slog.Error(err, "query deployment error")
		return err
	}

	// 如果错误是NotFound说明没有走更新逻辑 是第一次创建这个deployment 实例化一个数据结构
	deployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: webApp.Namespace,
			Name:      webApp.Name,
			Labels: map[string]string{
				"app": webApp.Name,
			},
		},
		Spec: appsv1.DeploymentSpec{
			// 副本数是计算出来的
			Replicas: webApp.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": webApp.Name,
				},
			},

			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": webApp.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: webApp.Name,
							// 用指定的镜像
							Image:           webApp.Spec.Image,
							ImagePullPolicy: "IfNotPresent",
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									Protocol:      corev1.ProtocolSCTP,
									ContainerPort: Port,
								},
							},
						},
					},
				},
			},
		},
	}

	if webApp.Spec.Command != nil {
		deployment.Spec.Template.Spec.Containers[0].Command = webApp.Spec.Command
	}

	if webApp.Spec.Args != nil {
		deployment.Spec.Template.Spec.Containers[0].Args = webApp.Spec.Args
	}

	if webApp.Spec.Env != nil {
		deployment.Spec.Template.Spec.Containers[0].Env = webApp.Spec.Env
	}

	if webApp.Spec.HostPathMount != nil {
		vlmms := []corev1.VolumeMount{}
		vlss := []corev1.Volume{}
		for i := range webApp.Spec.HostPathMount {
			fmt.Println(webApp.Spec.HostPathMount[i].Spath)
			fmt.Println(webApp.Spec.HostPathMount[i].Dpath)

			//创建Volume对象
			vlm := corev1.Volume{}
			vlm.HostPath = &corev1.HostPathVolumeSource{
				Path: webApp.Spec.HostPathMount[i].Dpath,
			}
			vlm.Name = "volume" + strconv.Itoa(i)
			vlss = append(vlss, vlm)

			//创建VolumeMount对象
			vlmm := corev1.VolumeMount{}
			vlmm.Name = "volume" + strconv.Itoa(i)
			vlmm.MountPath = webApp.Spec.HostPathMount[i].Spath
			vlmms = append(vlmms, vlmm)

		}
		deployment.Spec.Template.Spec.Volumes = vlss
		deployment.Spec.Template.Spec.Containers[0].VolumeMounts = vlmms

	}

	if webApp.Spec.HostBinding != nil {
		aff := corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "kubernetes.io/hostname",
									Operator: corev1.NodeSelectorOpIn,
									Values:   webApp.Spec.HostBinding,
								},
							},
						},
					},
				},
			},
		}
		deployment.Spec.Template.Spec.Affinity = &aff
	}

	// 这一步非常关键！
	// 建立关联后，删除elasticweb资源时就会将deployment也删除掉
	k8slog.Info("set reference")
	if err := controllerutil.SetControllerReference(webApp, deployment, r.Scheme); err != nil {
		k8slog.Error(err, "SetControllerReference error")
		return err
	}

	// 创建deployment
	k8slog.Info("start create deployment")
	if err := r.Create(ctx, deployment); err != nil {
		k8slog.Error(err, "create deployment error")
		return err
	}

	k8slog.Info("create deployment success")

	return nil

}

////创建Prometail-cm
func newPrometailCm(monitor *mobfunv1.WebApp) *corev1.ConfigMap {
	fmt.Println("打印prometail配置文件")
	pcm := monitor.Spec.Promtail.PromtailYml

	pcm = strings.Replace(pcm, "xxxproject", monitor.Name, 1)
	pcm = strings.Replace(pcm, "xxxnamespace", monitor.Namespace, 1)
	pcm = strings.Replace(pcm, "xxxproject", monitor.Name, 1)

	for i := range monitor.Spec.HostPathMount {
		if monitor.Spec.HostPathMount[i].DescribePath == "日志" {
			pcm = strings.Replace(pcm, "xxxpath", monitor.Spec.HostPathMount[i].Spath+"/logs/"+monitor.Name+"-k8s/*", 1)
		}
	}

	fmt.Println(pcm)

	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "prometail-cnf-" + monitor.Name,
			Namespace: monitor.Namespace,
		},
		Data: map[string]string{
			"promtail-config.yaml": pcm,
		},
	}
}

//创建Prometail
func newPrometailDaemonSet(ctx context.Context, r *WebAppReconciler, webApp *mobfunv1.WebApp, req ctrl.Request) error {
	k8slog := log.FromContext(ctx)
	daemonSet := &appsv1.DaemonSet{}

	k8slog.Info("start create Prometail daemonSet")

	daemonSet = &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: webApp.Namespace,
			Name:      webApp.Name + "-promtail",
			Labels: map[string]string{
				"app": webApp.Name,
			},
		},
		Spec: appsv1.DaemonSetSpec{

			// 副本数是计算出来的

			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": webApp.Name,
				},
			},

			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": webApp.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: webApp.Name + "-promtail",
							// 用指定的镜像
							Image:           webApp.Spec.Promtail.Image,
							ImagePullPolicy: "IfNotPresent",

							Args: []string{
								"-config.file=/etc/promtail-config/promtail-config.yaml",
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "prometail-cnf-" + webApp.Name,
									MountPath: "/etc/promtail-config",
								},
							},
						},
					},
				},
			},
		},
	}

	if webApp.Spec.HostPathMount != nil {
		vlmms := daemonSet.Spec.Template.Spec.Containers[0].VolumeMounts
		vlss := []corev1.Volume{}
		var iflog string = ""
		for i := range webApp.Spec.HostPathMount {
			if webApp.Spec.HostPathMount[i].DescribePath == "日志" {
				iflog = webApp.Spec.HostPathMount[i].DescribePath
				fmt.Println(webApp.Spec.HostPathMount[i].Spath)
				fmt.Println(webApp.Spec.HostPathMount[i].Dpath)

				//创建Volume对象
				vlm := corev1.Volume{}
				vlm.HostPath = &corev1.HostPathVolumeSource{
					Path: webApp.Spec.HostPathMount[i].Dpath,
				}
				vlm.Name = "volume" + strconv.Itoa(i)
				vlss = append(vlss, vlm)

				//创建VolumeMount对象
				vlmm := corev1.VolumeMount{}
				vlmm.Name = "volume" + strconv.Itoa(i)
				vlmm.MountPath = webApp.Spec.HostPathMount[i].Spath
				vlmms = append(vlmms, vlmm)
			}
		}

		if iflog != "日志" {
			k8slog.Info("没有配置日志采集目录，取消日志采集程序部署！")
			return nil
		}

		daemonSet.Spec.Template.Spec.Volumes = vlss
		daemonSet.Spec.Template.Spec.Containers[0].VolumeMounts = vlmms
		if !webApp.Spec.Promtail.IsEmpty() {
			vlmtail := corev1.Volume{
				Name: "prometail-cnf-" + webApp.Name,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "prometail-cnf-" + webApp.Name,
						},
						Items: []corev1.KeyToPath{
							{
								Key:  "promtail-config.yaml",
								Path: "promtail-config.yaml",
							},
						},
					},
				},
			}
			vlss = append(vlss, vlmtail)
			daemonSet.Spec.Template.Spec.Volumes = vlss

			depname := nas.NamespacedName{}
			depname.Namespace = webApp.Namespace
			depname.Name = "prometail-cnf-" + webApp.Name
			err := r.Get(ctx, depname, &corev1.ConfigMap{})
			if err == nil {
				k8slog.Info("prometail-cnf exists")
				return nil
			}
			cm := newPrometailCm(webApp)

			// 这一步非常关键！
			// 建立关联后，删除elasticweb资源时就会将deployment也删除掉
			k8slog.Info("set reference")
			if err := controllerutil.SetControllerReference(webApp, cm, r.Scheme); err != nil {
				k8slog.Error(err, "SetControllerReference error")
				return err
			}

			// 创建deployment
			k8slog.Info("start create promtailcm")
			if err := r.Create(ctx, cm); err != nil {
				k8slog.Error(err, "create promtailcm error")
				return err
			}

		}

	}

	// 这一步非常关键！
	// 建立关联后，删除elasticweb资源时就会将deployment也删除掉
	k8slog.Info("set reference")
	if err := controllerutil.SetControllerReference(webApp, daemonSet, r.Scheme); err != nil {
		k8slog.Error(err, "SetControllerReference error")
		return err
	}

	// 创建deployment
	k8slog.Info("start create daemonSet")
	if err := r.Create(ctx, daemonSet); err != nil {
		k8slog.Error(err, "create daemonSet error")
		return err
	}

	k8slog.Info("create daemonSet success")

	return nil

}

// 新建service
func createService(ctx context.Context, r *WebAppReconciler, webApp *mobfunv1.WebApp, req ctrl.Request) error {
	k8slog := log.FromContext(ctx)
	service := &corev1.Service{}

	err := r.Get(ctx, req.NamespacedName, service)

	if err == nil {
		k8slog.Info("service exists")
		return nil
	}

	// 如果错误不是NotFound，就返回错误
	if !errors.IsNotFound(err) {
		k8slog.Error(err, "query service error")
		return err
	}

	service = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: webApp.Namespace,
			Name:      webApp.Name,
			Labels: map[string]string{
				"app": webApp.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app": webApp.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:     "web-port",
					Protocol: corev1.ProtocolTCP,
					Port:     Port,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: Port,
					},
				},
			},
		},
	}

	// 这一步非常关键！
	// 建立关联后，删除webapp资源时就会将service也删除掉
	k8slog.Info("set reference")
	if err := controllerutil.SetControllerReference(webApp, service, r.Scheme); err != nil {
		k8slog.Error(err, "SetControllerReference error")
		return err
	}

	// 创建service
	k8slog.Info("start create service")
	if err := r.Create(ctx, service); err != nil {
		k8slog.Error(err, "create service error")
		return err
	}

	k8slog.Info("create service success")

	return nil

}

// 新建ingress
func createIngess(ctx context.Context, r *WebAppReconciler, webApp *mobfunv1.WebApp, req ctrl.Request) error {
	k8slog := log.FromContext(ctx)
	ingress := &ing.Ingress{}

	err := r.Get(ctx, req.NamespacedName, ingress)

	if err == nil {
		k8slog.Info("ingress exists")

		olding := oldIngress{}

		if err := json.Unmarshal([]byte(webApp.Annotations[oldIngressSpecAnnotation]), &olding); err != nil {
			return err
		}

		newing := oldIngress{
			HostName:    webApp.Spec.HostName,
			ContextPath: webApp.Spec.ContextPath,
		}

		//如果当前的状态和之前状态不一样则需要更新
		if !reflect.DeepEqual(newing, olding) {
			oldingress := &ing.Ingress{}
			if err := r.Get(ctx, req.NamespacedName, oldingress); err != nil {
				return err
			}

			oldingress.Spec.Rules[0].Host = newing.HostName
			oldingress.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Path = newing.ContextPath

			if err := r.Client.Update(ctx, oldingress); err != nil {
				return err
			}
		}

		return nil
	}

	// 如果错误不是NotFound，就返回错误
	if !errors.IsNotFound(err) {
		k8slog.Error(err, "query ingress error")
		return err
	}

	var ingclass = "nginx"
	var pathType = ing.PathTypePrefix
	ingress = &ing.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      webApp.Name,
			Namespace: webApp.Namespace,
			Labels: map[string]string{
				"app": webApp.Name,
			},
			Annotations: map[string]string{"nginx.ingress.kubernetes.io/server-snippet": ""},
		},
		Spec: ing.IngressSpec{
			IngressClassName: &ingclass,
			Rules: []ing.IngressRule{
				ing.IngressRule{
					webApp.Spec.HostName,
					ing.IngressRuleValue{
						HTTP: &ing.HTTPIngressRuleValue{
							Paths: []ing.HTTPIngressPath{
								ing.HTTPIngressPath{
									Path:     webApp.Spec.ContextPath,
									PathType: (&pathType),
									Backend: ing.IngressBackend{
										Service: &ing.IngressServiceBackend{
											Name: webApp.Name,
											Port: ing.ServiceBackendPort{
												Number: Port,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// 这一步非常关键！
	// 建立关联后，删除webapp资源时就会将service也删除掉
	k8slog.Info("set reference")
	if err := controllerutil.SetControllerReference(webApp, ingress, r.Scheme); err != nil {
		k8slog.Error(err, "SetControllerReference error")
		return err
	}

	// 创建ingress
	k8slog.Info("start create ingress")
	if err := r.Create(ctx, ingress); err != nil {
		k8slog.Error(err, "create ingress error")
		return err
	}

	k8slog.Info("create ingress success")

	return nil

}
