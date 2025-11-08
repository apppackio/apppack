package stacks

const (
	accountFormationURL         = "https://s3.amazonaws.com/apppack-cloudformations/latest/account.json"
	accountStackName            = "apppack-account"
	appFormationURL             = "https://s3.amazonaws.com/apppack-cloudformations/latest/app.json"
	AppStackNameTmpl            = "apppack-app-%s"
	clusterFormationURL         = "https://s3.amazonaws.com/apppack-cloudformations/latest/cluster.json"
	clusterStackNameTmpl        = "apppack-cluster-%s"
	customDomainFormationURL    = "https://s3.amazonaws.com/apppack-cloudformations/latest/custom-domain.json"
	customDomainStackNameTmpl   = "apppack-customdomain-%s"
	databaseStackNameTmpl       = "apppack-database-%s"
	databaseFormationURL        = "https://s3.amazonaws.com/apppack-cloudformations/latest/database.json"
	PipelineStackNameTmpl       = "apppack-pipeline-%s"
	redisFormationURL           = "https://s3.amazonaws.com/apppack-cloudformations/latest/redis.json"
	redisStackNameTmpl          = "apppack-redis-%s"
	redisAuthTokenParameterTmpl = "/apppack/redis/%s/auth-token" // #nosec G101 -- Parameter path template, not a credential
	regionFormationURL          = "https://s3.amazonaws.com/apppack-cloudformations/latest/region.json"
	regionStackNameTmpl         = "apppack-region-%s"
	reviewAppFormationURL       = "https://s3.amazonaws.com/apppack-cloudformations/latest/review-app.json"
	reviewAppStackNameTmpl      = "apppack-reviewapp-%s"
	Enabled                     = "enabled"
	DeleteComplete              = "DELETE_COMPLETE"
)
