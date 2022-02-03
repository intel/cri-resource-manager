package main

import (
	"context"
	"fmt"
	"os"
	"time"

	// Gardener
	"github.com/gardener/gardener/extensions/pkg/controller"
	extensionscontroller "github.com/gardener/gardener/extensions/pkg/controller"
	controllercmd "github.com/gardener/gardener/extensions/pkg/controller/cmd"
	"github.com/gardener/gardener/extensions/pkg/controller/extension"
	"github.com/gardener/gardener/extensions/pkg/controller/healthcheck"
	healthcheckconfig "github.com/gardener/gardener/extensions/pkg/controller/healthcheck/config"
	"github.com/gardener/gardener/extensions/pkg/controller/healthcheck/general"
	"github.com/gardener/gardener/extensions/pkg/util"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	resourcemanagerv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
	"github.com/gardener/gardener/pkg/logger"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	managedresources "github.com/gardener/gardener/pkg/utils/managedresources"

	// Other
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

const (
	ExtensionName = "cri-rm"
	ExtensionType = "cri-rm-extension"

	ControllerName = "cri-rm-controller"
	ActuatorName   = "cri-rm-actuator"

	ManagedResourceName = "extension-runtime-cri-rm"
	ConfigKey           = "config.yaml"

	ChartPath = "charts/example_configmap"
	// ChartPath               = "charts/cri-rm-installation/"
	InstallationImageName   = "installation_image_name"
	InstallationReleaseName = "cri-rm-installation"
)

func RegisterHealthChecks(mgr manager.Manager) error {
	defaultSyncPeriod := time.Second * 30
	opts := healthcheck.DefaultAddArgs{
		HealthCheckConfig: healthcheckconfig.HealthCheckConfig{SyncPeriod: metav1.Duration{Duration: defaultSyncPeriod}},
	}
	return healthcheck.DefaultRegistration(
		ExtensionType,
		extensionsv1alpha1.SchemeGroupVersion.WithKind(extensionsv1alpha1.ExtensionResource),
		func() client.ObjectList { return &extensionsv1alpha1.ExtensionList{} },
		func() extensionsv1alpha1.Object { return &extensionsv1alpha1.Extension{} },
		mgr,
		opts,
		nil,
		[]healthcheck.ConditionTypeToHealthCheck{
			{
				ConditionType: string(gardencorev1beta1.ShootSystemComponentsHealthy),
				HealthCheck:   general.CheckManagedResource(ManagedResourceName),
			},
		},
	)
}

type Options struct {
	restOptions       *controllercmd.RESTOptions
	managerOptions    *controllercmd.ManagerOptions
	controllerOptions *controllercmd.ControllerOptions
	reconcileOptions  *controllercmd.ReconcilerOptions
}

func main() {
	runtimelog.SetLogger(logger.ZapLogger(true))

	ctx := signals.SetupSignalHandler()

	options := &Options{
		restOptions: &controllercmd.RESTOptions{},
		managerOptions: &controllercmd.ManagerOptions{
			LeaderElection:          false,
			LeaderElectionID:        controllercmd.LeaderElectionNameID(ExtensionName),
			LeaderElectionNamespace: os.Getenv("LEADER_ELECTION_NAMESPACE"),
		},
		controllerOptions: &controllercmd.ControllerOptions{
			MaxConcurrentReconciles: 1,
		},
		reconcileOptions: &controllercmd.ReconcilerOptions{},
	}

	optionAggregator := controllercmd.NewOptionAggregator(
		options.restOptions,
		options.managerOptions,
		options.controllerOptions,
		options.reconcileOptions,
	)

	cmd := &cobra.Command{
		Use:   "cri-rm-controller-manager",
		Short: "CRI Resource manager Controller manages components which install CRI-Resource-Manager as CRI proxy.",

		RunE: func(cmd *cobra.Command, args []string) error {
			if err := optionAggregator.Complete(); err != nil {
				return fmt.Errorf("error completing options: %s", err)
			}

			mgrOpts := options.managerOptions.Completed().Options()
			mgrOpts.MetricsBindAddress = "0"

			// mgrOpts.ClientDisableCacheFor = []client.Object{
			// 	&corev1.Secret{},    // TODO: resolve race condition with small rsync time
			// }

			mgr, err := manager.New(options.restOptions.Completed().Config, mgrOpts)
			if err != nil {
				return fmt.Errorf("could not instantiate controller-manager: %s", err)
			}
			scheme := mgr.GetScheme()
			if err := extensionscontroller.AddToScheme(scheme); err != nil {
				return err
			}
			if err := resourcemanagerv1alpha1.AddToScheme(mgr.GetScheme()); err != nil {
				return err
			}

			// TODO: enable healthcheck
			// if err := RegisterHealthChecks(mgr); err != nil {
			// 	return err
			// }

			// For development purposes.
			ignoreOperationAnnotation := false

			if err := extension.Add(mgr, extension.AddArgs{
				Actuator:                  NewActuator(),
				ControllerOptions:         options.controllerOptions.Completed().Options(),
				Name:                      ControllerName,
				FinalizerSuffix:           ExtensionType,
				Resync:                    60 * time.Minute, // was 60 // FIXME: with 1 minute resync we have race condition during deletion
				Predicates:                extension.DefaultPredicates(ignoreOperationAnnotation),
				Type:                      ExtensionType,
				IgnoreOperationAnnotation: ignoreOperationAnnotation,
			}); err != nil {
				return fmt.Errorf("error configuring actuator: %s", err)
			}

			if err := mgr.Start(ctx); err != nil {
				return fmt.Errorf("error running manager: %s", err)
			}

			return nil
		},
	}

	optionAggregator.AddFlags(cmd.Flags())

	if err := cmd.ExecuteContext(ctx); err != nil {
		controllercmd.LogErrAndExit(err, "error executing the main controller command")
	}

}

func NewActuator() extension.Actuator {
	return &actuator{
		chartRendererFactory: extensionscontroller.ChartRendererFactoryFunc(util.NewChartRendererForShoot),
		logger:               log.Log.WithName(ActuatorName),
	}
}

type actuator struct {
	client               client.Client
	config               *rest.Config
	chartRendererFactory extensionscontroller.ChartRendererFactory
	decoder              runtime.Decoder
	logger               logr.Logger
}

func (a *actuator) Reconcile(ctx context.Context, ex *extensionsv1alpha1.Extension) error {

	// Find what shoot cluster we dealing with.
	namespace := ex.GetNamespace()
	cluster, err := controller.GetCluster(ctx, a.client, namespace)
	if err != nil {
		return err
	}
	a.logger.Info("Reconcile: checking extension...") // , "shoot", cluster.Shoot.Name, "namespace", cluster.Shoot.Namespace)

	// TO handle deletion timestamp.
	mr := &resourcemanagerv1alpha1.ManagedResource{}
	if err := a.client.Get(ctx, kutil.Key(namespace, ManagedResourceName), mr); err != nil {
		// Continue only if not found.
		if !apierrors.IsNotFound(err) {
			return err
		}

		a.logger.Info("Reconcile: installing extension (managedresource)...") // , "shoot", cluster.Shoot.Name, "namespace", cluster.Shoot.Namespace)
		// Depending on shoot, chartredner will have different capabilities based on K8s version..
		chartRenderer, err := a.chartRendererFactory.NewChartRendererForShoot(cluster.Shoot.Spec.Kubernetes.Version)

		chartValues := map[string]interface{}{
			"images": map[string]string{
				InstallationImageName: "foo", // TODO: imagevector.FindImage(InstallationImageName),
			},
		}

		release, err := chartRenderer.Render(ChartPath, InstallationReleaseName, metav1.NamespaceSystem, chartValues)
		if err != nil {
			return err
		}

		// Put chart into secret
		secretData := map[string][]byte{ConfigKey: release.Manifest()}

		// Reconcile managedresource and secret for shoot.
		return managedresources.CreateForShoot(ctx, a.client, namespace, ManagedResourceName, false, secretData)
	} else {
		a.logger.Info("Reconcile: extension is already installed. Ignoring.") //, "shoot", cluster.Shoot.Name, "namespace", cluster.Shoot.Namespace)
	}

	return nil
}

func (a *actuator) Delete(ctx context.Context, ex *extensionsv1alpha1.Extension) error {
	namespace := ex.GetNamespace()
	cluster, err := controller.GetCluster(ctx, a.client, namespace)
	if err != nil {
		return err
	}
	a.logger.Info("Delete: deleting extension managedresources in shoot", "shoot", cluster.Shoot.Name, "namespace", cluster.Shoot.Namespace)

	twoMinutes := 1 * time.Minute

	timeoutShootCtx, cancelShootCtx := context.WithTimeout(ctx, twoMinutes)
	defer cancelShootCtx()

	if err := managedresources.DeleteForShoot(ctx, a.client, namespace, ManagedResourceName); err != nil {
		return err
	}

	if err := managedresources.WaitUntilDeleted(timeoutShootCtx, a.client, namespace, ManagedResourceName); err != nil {
		return err
	}

	return nil
}

func (a *actuator) Restore(ctx context.Context, ex *extensionsv1alpha1.Extension) error {
	return a.Reconcile(ctx, ex)
}

func (a *actuator) Migrate(ctx context.Context, ex *extensionsv1alpha1.Extension) error {
	return a.Delete(ctx, ex)
}

func (a *actuator) InjectConfig(config *rest.Config) error {
	a.config = config
	return nil
}

func (a *actuator) InjectClient(client client.Client) error {
	a.client = client
	return nil
}

func (a *actuator) InjectScheme(scheme *runtime.Scheme) error {
	a.decoder = serializer.NewCodecFactory(scheme, serializer.EnableStrict).UniversalDecoder()
	return nil
}
