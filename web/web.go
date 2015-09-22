package web

import (
	"github.com/urandom/readeef"
	"github.com/urandom/webfw"
	"github.com/urandom/webfw/context"
	"github.com/urandom/webfw/middleware"
	"github.com/urandom/webfw/renderer"
)

func RegisterControllers(config readeef.Config, dispatcher *webfw.Dispatcher, apiPattern string) {
	dispatcher.Renderer = renderer.NewRenderer(dispatcher.Config.Renderer.Dir,
		dispatcher.Config.Renderer.Base)

	dispatcher.Renderer.Delims("{%", "%}")
	dispatcher.Context.SetGlobal(readeef.CtxKey("config"), config)
	dispatcher.Context.SetGlobal(context.BaseCtxKey("readeefConfig"), config)

	middleware.InitializeDefault(dispatcher)

	dispatcher.Handle(NewApp())
	dispatcher.Handle(NewComponent(dispatcher, apiPattern))

	hasProxy := false
	for _, p := range config.FeedParser.Processors {
		if p == "proxy-http" {
			hasProxy = true
			break
		}
	}

	if !hasProxy {
		for _, p := range config.Content.ArticleProcessors {
			if p == "proxy-http" {
				hasProxy = true
				break
			}
		}
	}

	if hasProxy {
		dispatcher.Handle(NewProxy())
	}
}
