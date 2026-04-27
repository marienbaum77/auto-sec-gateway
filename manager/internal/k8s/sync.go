package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/marienbaum77/auto-sec-gateway/internal/model"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	namespace      = "prod"
	configMapName  = "xray-config"
	configJSONKey  = "config.json"
	deploymentName = "xray"
)

type xrayClient struct {
	ID    string `json:"id"`
	Email string `json:"email,omitempty"`
}

// SyncXrayConfig rewrites Xray clients list from active DB users and restarts deployment.
func SyncXrayConfig(ctx context.Context, db *gorm.DB) error {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("build in-cluster config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("build kubernetes client: %w", err)
	}

	cmClient := clientset.CoreV1().ConfigMaps(namespace)
	cm, err := cmClient.Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get configmap %s/%s: %w", namespace, configMapName, err)
	}

	rawConfig, ok := cm.Data[configJSONKey]
	if !ok {
		return fmt.Errorf("configmap %s/%s does not have %q", namespace, configMapName, configJSONKey)
	}

	var config map[string]any
	if err := json.Unmarshal([]byte(rawConfig), &config); err != nil {
		return fmt.Errorf("parse xray config json: %w", err)
	}

	var users []model.User
	if err := db.Where("active = ?", true).Find(&users).Error; err != nil {
		return fmt.Errorf("load active users: %w", err)
	}

	clients := make([]any, 0, len(users))
	for _, user := range users {
		clients = append(clients, xrayClient{
			ID:    user.UUID,
			Email: user.Username,
		})
	}

	if err := setClients(config, clients); err != nil {
		return err
	}

	updatedConfig, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal updated xray config: %w", err)
	}

	cmCopy := cm.DeepCopy()
	if cmCopy.Data == nil {
		cmCopy.Data = map[string]string{}
	}
	cmCopy.Data[configJSONKey] = string(updatedConfig)

	if _, err := cmClient.Update(ctx, cmCopy, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update configmap %s/%s: %w", namespace, configMapName, err)
	}

	if err := restartDeployment(ctx, clientset); err != nil {
		return err
	}

	return nil
}

func setClients(config map[string]any, clients []any) error {
	inboundsRaw, ok := config["inbounds"].([]any)
	if !ok || len(inboundsRaw) == 0 {
		return fmt.Errorf("xray config has no inbounds[0]")
	}

	firstInbound, ok := inboundsRaw[0].(map[string]any)
	if !ok {
		return fmt.Errorf("xray config inbound[0] has invalid format")
	}

	settings, ok := firstInbound["settings"].(map[string]any)
	if !ok {
		settings = map[string]any{}
		firstInbound["settings"] = settings
	}

	settings["clients"] = clients
	return nil
}

func restartDeployment(ctx context.Context, clientset *kubernetes.Clientset) error {
	patch := map[string]any{
		"spec": map[string]any{
			"template": map[string]any{
				"metadata": map[string]any{
					"annotations": map[string]string{
						"kubectl.kubernetes.io/restartedAt": time.Now().UTC().Format(time.RFC3339),
					},
				},
			},
		},
	}

	payload, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshal deployment restart patch: %w", err)
	}

	_, err = clientset.AppsV1().Deployments(namespace).Patch(
		ctx,
		deploymentName,
		types.MergePatchType,
		payload,
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("patch deployment %s/%s restart annotation: %w", namespace, deploymentName, err)
	}

	return nil
}
