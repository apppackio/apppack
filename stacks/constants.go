package stacks

const (
	appFormationURL             = "https://s3.amazonaws.com/apppack-cloudformations/latest/app.json"
	clusterFormationURL         = "https://s3.amazonaws.com/apppack-cloudformations/latest/cluster.json"
	accountFormationURL         = "https://s3.amazonaws.com/apppack-cloudformations/latest/account.json"
	regionFormationURL          = "https://s3.amazonaws.com/apppack-cloudformations/latest/region.json"
	databaseFormationURL        = "https://s3.amazonaws.com/apppack-cloudformations/latest/database.json"
	redisFormationURL           = "https://s3.amazonaws.com/apppack-cloudformations/latest/redis.json"
	customDomainFormationURL    = "https://s3.amazonaws.com/apppack-cloudformations/latest/custom-domain.json"
	accountStackName            = "apppack-account"
	appStackNameTmpl            = "apppack-app-%s"
	pipelineStackNameTmpl       = "apppack-pipeline-%s"
	redisStackNameTmpl          = "apppack-redis-%s"
	redisAuthTokenParameterTmpl = "/apppack/redis/%s/auth-token"
	databaseStackNameTmpl       = "apppack-database-%s"
	clusterStackNameTmpl        = "apppack-cluster-%s"
)
