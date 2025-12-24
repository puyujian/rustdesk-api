package web

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
)

type Index struct {
}

func (i *Index) Index(c *gin.Context) {
	// 使用前端 JS 保留 hash 路由（支付 return_url 常见为 http(s)://host/#/...）
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(200, `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Redirecting...</title></head><body><script>(function(){var hash=window.location.hash||'';window.location.replace('/_admin/'+hash);})();</script><noscript><meta http-equiv="refresh" content="0;url=/_admin/"><a href="/_admin/">Continue</a></noscript></body></html>`)
}

func (i *Index) ConfigJs(c *gin.Context) {
	apiServer := global.Config.Rustdesk.ApiServer
	magicQueryonline := global.Config.Rustdesk.WebclientMagicQueryonline
	tmp := fmt.Sprintf(`localStorage.setItem('api-server', '%v');
const ws2_prefix = 'wc-';
localStorage.setItem(ws2_prefix+'api-server', '%v');

window.webclient_magic_queryonline = %d;
window.ws_host = '%v';
`, apiServer, apiServer, magicQueryonline, global.Config.Rustdesk.WsHost)
	//	tmp := `
	//localStorage.setItem('api-server', "` + apiServer + `")
	//const ws2_prefix = 'wc-'
	//localStorage.setItem(ws2_prefix+'api-server', "` + apiServer + `")
	//
	//window.webclient_magic_queryonline = ` + magicQueryonline + ``

	c.Header("Content-Type", "application/javascript")
	c.String(200, tmp)
}
