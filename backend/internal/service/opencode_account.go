package service

import opencodepkg "github.com/Wei-Shaw/sub2api/internal/pkg/opencode"

// ResolveOpenCodeModel applies account model mapping and model-level protocol
// overrides using the same rules as the OpenCode forwarding service.
func ResolveOpenCodeModel(account *Account, requestedModel string) (opencodepkg.ModelResolution, error) {
	billingModel := opencodepkg.NormalizeModelID(requestedModel)
	mappedModel := billingModel
	if account != nil {
		if value, matched := account.ResolveMappedModel(requestedModel); matched {
			mappedModel = value
		} else if billingModel != requestedModel {
			mappedModel = account.GetMappedModel(billingModel)
		}
	}
	var overrides any
	if account != nil {
		overrides = account.GetModelProtocols()
	}
	return opencodepkg.ResolveModel(requestedModel, mappedModel, overrides)
}

func (a *Account) IsOpenCodeModelSupported(requestedModel string) bool {
	if a == nil || !a.IsOpenCodeAPIKey() {
		return false
	}
	_, err := ResolveOpenCodeModel(a, requestedModel)
	return err == nil
}
