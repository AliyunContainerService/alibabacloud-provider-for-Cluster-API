package cs

import (
	"github.com/crossplane/upjet/pkg/config"
)

var managedKubernetesRemovedAttrs = []string{
	"name_prefix", // conflicted with name
	"worker_instance_types",
	"worker_number",
	"worker_disk_size",
	"worker_disk_category",
	"worker_disk_performance_level",
	"worker_disk_snapshot_policy_id",
	"worker_data_disk_size",
	"worker_data_disk_category",
	"worker_instance_charge_type",
	"worker_data_disks",
	"worker_period_unit",
	"worker_period",
	"worker_auto_renew",
	"worker_auto_renew_period",
	"exclude_autoscaler_nodes",
	"enable_ssh",
	"password",
	"key_name",
	"kms_encrypted_password",
	"kms_encryption_context",
	"image_id",
	"install_cloud_monitor",
	"cpu_policy",
	"os_type",
	"platform",
	"node_port_range",
	"runtime",
	"taints",
	"rds_instances",
	"user_data",
	"node_name_mode",
	"worker_nodes",
	"kube_config",
	"availability_zone",
	"vswitch_ids",
	"force_update",
	"worker_numbers",
	"cluster_network_type",
	"log_config",
	"worker_instance_type",
}

var nodePoolRemovedAttrs = []string{
	"node_count",
	"security_group_id",
	"platform",
	"rollout_policy",
}

func Configure(p *config.Provider) {
	p.AddResourceConfigurator("alicloud_cs_managed_kubernetes", func(r *config.Resource) {
		for _, attr := range managedKubernetesRemovedAttrs {
			delete(r.TerraformResource.Schema, attr)
		}

		//r.TerraformResource.Schema["region"] = &schema.Schema{
		//	Type:                  schema.TypeString,
		//	ConfigMode:            0,
		//	Required:              false,
		//	Optional:              false,
		//	Computed:              false,
		//	ForceNew:              true,
		//	DiffSuppressFunc:      nil,
		//	DiffSuppressOnRefresh: false,
		//	Default:               nil,
		//	DefaultFunc:           nil,
		//	Description:           "",
		//	InputDefault:          "",
		//	StateFunc:             nil,
		//	Elem:                  nil,
		//	MaxItems:              0,
		//	MinItems:              0,
		//	Set:                   nil,
		//	ComputedWhen:          nil,
		//	ConflictsWith:         nil,
		//	ExactlyOneOf:          nil,
		//	AtLeastOneOf:          nil,
		//	RequiredWith:          nil,
		//	Deprecated:            "",
		//	ValidateFunc:          nil,
		//	ValidateDiagFunc:      nil,
		//	Sensitive:             false,
		//}
	})
	p.AddResourceConfigurator("alicloud_cs_kubernetes_node_pool", func(r *config.Resource) {
		for _, attr := range nodePoolRemovedAttrs {
			delete(r.TerraformResource.Schema, attr)
		}
	})

	p.AddResourceConfigurator("alicloud_vpc", func(r *config.Resource) {
		delete(r.TerraformResource.Schema, "name")
	})

	p.AddResourceConfigurator("alicloud_vswitch", func(r *config.Resource) {
		delete(r.TerraformResource.Schema, "availability_zone")
		delete(r.TerraformResource.Schema, "name")
	})
}
