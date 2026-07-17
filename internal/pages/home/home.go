package home

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"html/template"
	mathrand "math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/junfuchang/superflare/config/data"
	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/auth"
	"github.com/junfuchang/superflare/internal/background"
	"github.com/junfuchang/superflare/internal/fn"
	"github.com/junfuchang/superflare/internal/footer"
	"github.com/junfuchang/superflare/internal/i18n"
	"github.com/junfuchang/superflare/internal/pool"
	"github.com/junfuchang/superflare/internal/statuspage"
)

type homeRuntimeSnapshot struct {
	DebugMode  bool
	DisableCSP bool
	Visibility string
}

type homeRuntimeHolder struct {
	mu  sync.RWMutex
	set bool
	cfg homeRuntimeSnapshot
}

func (h *homeRuntimeHolder) Load() homeRuntimeSnapshot {
	if h == nil {
		return homeRuntimeSnapshot{}
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	if !h.set {
		return homeRuntimeSnapshot{}
	}
	return h.cfg
}

func (h *homeRuntimeHolder) Store(cfg homeRuntimeSnapshot) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.set = true
	h.cfg = cfg
	h.mu.Unlock()
}

var homeRuntimeFlags = &homeRuntimeHolder{}

func homeRuntimeSnapshotFromFlags(flags model.Flags) homeRuntimeSnapshot {
	return homeRuntimeSnapshot{
		DebugMode:  flags.DebugMode,
		DisableCSP: flags.DisableCSP,
		Visibility: strings.TrimSpace(flags.Visibility),
	}
}

func StoreRuntimeFlags(flags model.Flags) {
	homeRuntimeFlags.Store(homeRuntimeSnapshotFromFlags(flags))
}

func currentHomeRuntime() homeRuntimeSnapshot {
	homeRuntimeFlags.mu.RLock()
	hasValue := homeRuntimeFlags.set
	cfg := homeRuntimeFlags.cfg
	homeRuntimeFlags.mu.RUnlock()
	if hasValue {
		return cfg
	}
	cfg = homeRuntimeSnapshotFromFlags(define.CurrentAppRuntimeFlags())
	homeRuntimeFlags.Store(cfg)
	return cfg
}

const _cspValue = "object-src 'none'; base-uri 'none'; require-trusted-types-for 'script';"
const _cspScriptNone = "script-src 'none'; "
const _inlineClockScript = `(function(){var container=document.getElementById("live-datetime");if(!container){return;}var dateNode=container.querySelector('[data-role="date"]');var dayNode=container.querySelector('[data-role="day"]');var timeNode=container.querySelector('[data-role="time"]');var locale=container.getAttribute("data-locale")||"zh";var browserLocale=locale==="en"?"en-US":"zh-CN";function formatDate(now){if(locale==="en"){return new Intl.DateTimeFormat(browserLocale,{month:"short",day:"2-digit",year:"numeric"}).format(now);}return new Intl.DateTimeFormat(browserLocale,{year:"numeric",month:"long",day:"numeric"}).format(now);}function formatDay(now){return new Intl.DateTimeFormat(browserLocale,{weekday:"long"}).format(now);}function pad(num){return String(num).padStart(2,"0");}function tick(){var now=new Date();if(dateNode){dateNode.textContent=formatDate(now);}if(dayNode){dayNode.textContent=formatDay(now);}if(timeNode){timeNode.textContent=[pad(now.getHours()),pad(now.getMinutes()),pad(now.getSeconds())].join(":");}}tick();window.setInterval(tick,1000);}());`
const _inlineBackgroundLoaderScript = background.InlineLoaderScript
const _inlineSiteIconRefreshScript = `(function(){var directNodes=Array.prototype.slice.call(document.querySelectorAll("img[data-site-icon-direct][data-site-icon-fallback-src]"));directNodes.forEach(function(img){function useFallback(){var fallback=img.dataset.siteIconFallbackSrc;if(!fallback||img.dataset.siteIconFallbackDone==="1"){return;}img.dataset.siteIconFallbackDone="1";img.removeAttribute("data-site-icon-direct");img.src=fallback;}img.addEventListener("error",useFallback,{once:true});if(img.complete&&img.naturalWidth===0){useFallback();}});var nodes=Array.prototype.slice.call(document.querySelectorAll("[data-site-icon-src]"));if(!nodes.length||!window.fetch||!window.URL||!URL.createObjectURL){return;}var groups=new Map();nodes.forEach(function(node){var src=node.dataset.siteIconSrc;if(!src){return;}var group=groups.get(src);if(group){group.push(node);}else{groups.set(src,[node]);}});var objectUrls=[];groups.forEach(function(group,src){fetch(src).then(function(res){if(!res.ok||res.headers.get("X-SuperFlare-Site-Icon")!=="cached"){return null;}return res.blob();}).then(function(blob){if(!blob){return;}var objectUrl=URL.createObjectURL(blob);var probe=new Image();var settled=false;function apply(){if(settled){return;}settled=true;objectUrls.push(objectUrl);group.forEach(function(node){if(node.tagName==="IMG"){node.src=objectUrl;node.dataset.siteIconDone="1";return;}var img=document.createElement("img");img.src=objectUrl;img.alt="";img.dataset.siteIconDone="1";node.replaceWith(img);});}function discard(){if(settled){return;}settled=true;URL.revokeObjectURL(objectUrl);}if(typeof probe.decode==="function"){probe.src=objectUrl;probe.decode().then(apply,discard);}else{probe.onload=apply;probe.onerror=discard;probe.src=objectUrl;}}).catch(function(){});});window.addEventListener("pagehide",function(){objectUrls.forEach(function(objectUrl){URL.revokeObjectURL(objectUrl);});},{once:true});}());`
const _inlineBookmarkTooltipScript = `(function(){var selector="[data-bookmark-description]";var tooltipId="bookmark-description-tooltip";var tooltip=document.getElementById(tooltipId);if(!tooltip){tooltip=document.createElement("div");tooltip.id=tooltipId;tooltip.className="bookmark-description-tooltip";tooltip.setAttribute("role","tooltip");tooltip.hidden=true;document.body.appendChild(tooltip);}var hoverTarget=null;var hoverReady=false;var hoverTimer=null;var focusTarget=null;var activeTarget=null;var originalAriaDescribedBy=null;function cancelHoverTimer(){if(hoverTimer!==null){window.clearTimeout(hoverTimer);hoverTimer=null;}}function clearHover(){cancelHoverTimer();hoverTarget=null;hoverReady=false;}function restoreDescription(){if(!activeTarget){return;}if(originalAriaDescribedBy===null){activeTarget.removeAttribute("aria-describedby");}else{activeTarget.setAttribute("aria-describedby",originalAriaDescribedBy);}activeTarget=null;originalAriaDescribedBy=null;}function hideRendered(){restoreDescription();tooltip.hidden=true;tooltip.textContent="";}function position(target){var gap=8;var targetRect=target.getBoundingClientRect();var tooltipRect=tooltip.getBoundingClientRect();var left=targetRect.left+(targetRect.width-tooltipRect.width)/2;var top=targetRect.bottom+gap;if(top+tooltipRect.height>window.innerHeight-gap){top=targetRect.top-tooltipRect.height-gap;}left=Math.max(gap,Math.min(left,window.innerWidth-tooltipRect.width-gap));top=Math.max(gap,Math.min(top,window.innerHeight-tooltipRect.height-gap));tooltip.style.left=Math.round(left)+"px";tooltip.style.top=Math.round(top)+"px";}function show(target){if(!target||!target.isConnected){hideRendered();return;}var description=target.getAttribute("data-bookmark-description");if(!description||!description.trim()){hideRendered();return;}if(activeTarget!==target){restoreDescription();if(!target.isConnected){hideRendered();return;}activeTarget=target;originalAriaDescribedBy=target.getAttribute("aria-describedby");var describedBy=originalAriaDescribedBy?originalAriaDescribedBy.trim().split(/\s+/):[];if(describedBy.indexOf(tooltipId)===-1){describedBy.push(tooltipId);}target.setAttribute("aria-describedby",describedBy.join(" "));}tooltip.textContent=description;tooltip.hidden=false;position(target);}function reconcile(){if(hoverTarget&&!hoverTarget.isConnected){clearHover();}if(focusTarget&&!focusTarget.isConnected){focusTarget=null;}var target=focusTarget||(hoverReady?hoverTarget:null);if(!target){hideRendered();return;}show(target);}function reset(){clearHover();focusTarget=null;hideRendered();}function findTarget(event){var origin=event.target;if(!origin||!origin.closest){return null;}return origin.closest(selector);}function movedWithin(target,relatedTarget){return relatedTarget&&target.contains(relatedTarget);}document.addEventListener("pointerover",function(event){var target=findTarget(event);if(!target||movedWithin(target,event.relatedTarget)){return;}clearHover();hoverTarget=target;var scheduledTarget=target;hoverTimer=window.setTimeout(function(){hoverTimer=null;if(hoverTarget!==scheduledTarget||!scheduledTarget.isConnected){if(hoverTarget===scheduledTarget){hoverTarget=null;hoverReady=false;}reconcile();return;}hoverReady=true;reconcile();},500);reconcile();});document.addEventListener("pointerout",function(event){var target=findTarget(event);if(!target||movedWithin(target,event.relatedTarget)){return;}if(target===hoverTarget){clearHover();reconcile();}});document.addEventListener("focusin",function(event){var target=findTarget(event);if(target&&target.isConnected){focusTarget=target;reconcile();}});document.addEventListener("focusout",function(event){var target=findTarget(event);if(!target||movedWithin(target,event.relatedTarget)){return;}if(target===focusTarget){focusTarget=null;reconcile();}});function resetOnActivation(event){if(findTarget(event)){reset();}}document.addEventListener("click",resetOnActivation,true);document.addEventListener("auxclick",resetOnActivation,true);function cleanupDisconnectedTargets(){var changed=false;if(hoverTarget&&!hoverTarget.isConnected){clearHover();changed=true;}if(focusTarget&&!focusTarget.isConnected){focusTarget=null;changed=true;}if(activeTarget&&!activeTarget.isConnected){changed=true;}if(changed){reconcile();}}var observer=new MutationObserver(cleanupDisconnectedTargets);observer.observe(document.body,{childList:true,subtree:true});window.addEventListener("scroll",reset,true);document.addEventListener("keydown",function(event){if(event.key==="Escape"){reset();}});document.addEventListener("visibilitychange",reset);window.addEventListener("pagehide",reset);}());`
const _inlineApplicationSubdirectoryModalScript = `(function(){
var modalSelector=".application-subdirectory-modal";
var triggerSelector=".application-subdirectory-trigger a[aria-controls]";
var closeSelector=".application-subdirectory-close,.application-subdirectory-backdrop";
var panelSelector=".application-subdirectory-panel";
var inertMarker="data-application-subdirectory-inert";
var activeModal=null;
var lastTrigger=null;
function triggers(){return Array.prototype.slice.call(document.querySelectorAll(triggerSelector));}
function modalFromHash(){
if(!window.location.hash||window.location.hash==="#"){return null;}
var id=window.location.hash.slice(1);
if(!/^application-subdir-modal-\d+$/.test(id)){return null;}
var candidate=document.getElementById(id);
return candidate&&candidate.matches(modalSelector)?candidate:null;
}
function panelFor(modal){return modal?modal.querySelector(panelSelector):null;}
function findTrigger(modal){
if(!modal){return null;}
var found=null;
triggers().some(function(trigger){
if(trigger.getAttribute("aria-controls")===modal.id){found=trigger;return true;}
return false;
});
return found;
}
function setExpanded(modal){
triggers().forEach(function(trigger){
trigger.setAttribute("aria-expanded",modal&&trigger.getAttribute("aria-controls")===modal.id?"true":"false");
});
}
function setBackgroundInert(modal){
Array.prototype.slice.call(document.body.children).forEach(function(child){
if(child===modal||child.tagName==="SCRIPT"||child.hasAttribute("inert")){return;}
child.setAttribute(inertMarker,"");
child.setAttribute("inert","");
});
}
function restoreBackground(){
Array.prototype.slice.call(document.querySelectorAll("["+inertMarker+"]")).forEach(function(child){
child.removeAttribute("inert");
child.removeAttribute(inertMarker);
});
}
function focusPanel(modal){
var panel=panelFor(modal);
if(!panel){return;}
window.requestAnimationFrame(function(){
if(activeModal===modal){panel.focus({preventScroll:true});}
});
}
function restoreTrigger(){
var trigger=lastTrigger;
lastTrigger=null;
if(!trigger||!trigger.isConnected){return;}
window.setTimeout(function(){trigger.focus({preventScroll:true});},0);
}
function syncModal(){
var next=modalFromHash();
if(next===activeModal){return;}
restoreBackground();
if(next&&!lastTrigger){lastTrigger=findTrigger(next);}
activeModal=next;
setExpanded(activeModal);
if(activeModal){setBackgroundInert(activeModal);focusPanel(activeModal);}else{restoreTrigger();}
}
function closeActiveModal(){
if(!activeModal){return;}
window.location.hash="";
}
function focusableElements(modal){
var panel=panelFor(modal);
if(!panel){return [];}
return Array.prototype.slice.call(panel.querySelectorAll('a[href],button:not([disabled]),input:not([disabled]),select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"])')).filter(function(node){
return node.getClientRects().length>0&&!node.hasAttribute("disabled");
});
}
document.addEventListener("click",function(event){
var origin=event.target;
if(!origin||!origin.closest){return;}
var closeControl=origin.closest(closeSelector);
if(closeControl&&activeModal&&activeModal.contains(closeControl)){event.preventDefault();closeActiveModal();return;}
var trigger=origin.closest(triggerSelector);
if(trigger){lastTrigger=trigger;}
});
document.addEventListener("keydown",function(event){
if(!activeModal){return;}
if(event.key==="Escape"){event.preventDefault();closeActiveModal();return;}
if(event.key!=="Tab"){return;}
var panel=panelFor(activeModal);
if(!panel){return;}
var items=focusableElements(activeModal);
if(!items.length){event.preventDefault();panel.focus({preventScroll:true});return;}
var first=items[0];
var last=items[items.length-1];
var focused=document.activeElement;
if(event.shiftKey){
if(focused===first||focused===panel||!activeModal.contains(focused)){event.preventDefault();last.focus();}
}else if(focused===last||!activeModal.contains(focused)){event.preventDefault();first.focus();}
});
window.addEventListener("hashchange",syncModal);
syncModal();
}());`

var cryptoRandRead = rand.Read

func setCSPHeader(c *echo.Context, scriptNonce string) {
	if !currentHomeRuntime().DisableCSP {
		c.Response().Header().Set("Content-Security-Policy", getCSPValue(scriptNonce))
	}
}

func customHomeStyle(options model.Application, assets background.Assets) template.HTML {
	var b strings.Builder
	hasBackground := assets.Enabled
	if assets.Enabled {
		opacity := options.BackgroundOpacity
		if opacity < 0 {
			opacity = 0
		}
		if opacity > 100 {
			opacity = 100
		}
		blur := options.BackgroundBlur
		if blur < 0 {
			blur = 0
		}
		b.WriteString(`<style>.page-background{position:fixed;inset:0;z-index:-1;pointer-events:none;overflow:hidden;}.page-background img{position:absolute;inset:0;width:100%;height:100%;object-fit:cover;object-position:center center;transform:scale(1.08);filter:blur(`)
		b.WriteString(strconv.Itoa(blur))
		b.WriteString(`px);opacity:`)
		b.WriteString(strconv.FormatFloat(float64(opacity)/100, 'f', 2, 64))
		b.WriteString(`;}body.has-preview-background,body.has-loaded-background{background-image:none !important;}.page-background-preview{opacity:0;}.page-background.has-preview .page-background-preview{opacity:`)
		b.WriteString(strconv.FormatFloat(float64(opacity)/100, 'f', 2, 64))
		b.WriteString(`;}.page-background-full{opacity:0;}.page-background.is-loaded .page-background-preview{opacity:0;}.page-background.is-loaded .page-background-full{opacity:`)
		b.WriteString(strconv.FormatFloat(float64(opacity)/100, 'f', 2, 64))
		b.WriteString(`;}.page-background.is-failed .page-background-full{opacity:0;}</style>`)
		if assets.AccentColor != "" {
			b.WriteString(`<style>body{--scrollbar-accent:`)
			b.WriteString(assets.AccentColor)
			b.WriteString(`;}</style>`)
		}
	}
	if options.HomeMaxWidth > 0 {
		b.WriteString(`<style>#page-home.pageview .container{max-width:`)
		b.WriteString(strconv.Itoa(options.HomeMaxWidth))
		b.WriteString(`px;}</style>`)
	}
	if options.HomeMaxColumns > 0 {
		appendAdaptiveColumnStyle(&b, options.HomeMaxColumns)
	}
	if options.GlassEffect != "" && options.GlassEffect != "none" && options.GlassIntensity > 0 {
		intensity := options.GlassIntensity
		if intensity > 100 {
			intensity = 100
		}
		blur := 6 + intensity/4
		tintAlpha := 0.06 + float64(intensity)/520
		highlightAlpha := 0.12 + float64(intensity)/480
		if !hasBackground {
			b.WriteString(`<style>body.glass-frosted,body.glass-liquid{background-image:radial-gradient(circle at top,rgba(255,255,255,`)
			b.WriteString(strconv.FormatFloat(tintAlpha, 'f', 3, 64))
			b.WriteString(`),transparent 58%),linear-gradient(180deg,rgba(255,255,255,`)
			b.WriteString(strconv.FormatFloat(tintAlpha/1.2, 'f', 3, 64))
			b.WriteString(`),transparent 72%);}</style>`)
		} else {
			b.WriteString(`<style>.page-background::after{content:"";position:absolute;inset:0;background:linear-gradient(180deg,rgba(255,255,255,`)
			b.WriteString(strconv.FormatFloat(tintAlpha, 'f', 3, 64))
			b.WriteString(`),rgba(255,255,255,`)
			b.WriteString(strconv.FormatFloat(tintAlpha/1.6, 'f', 3, 64))
			b.WriteString(`));backdrop-filter:blur(`)
			b.WriteString(strconv.Itoa(blur))
			b.WriteString(`px);-webkit-backdrop-filter:blur(`)
			b.WriteString(strconv.Itoa(blur))
			b.WriteString(`px);}body.glass-liquid .page-background::after{background:radial-gradient(circle at top left,rgba(255,255,255,`)
			b.WriteString(strconv.FormatFloat(highlightAlpha, 'f', 3, 64))
			b.WriteString(`),transparent 38%),linear-gradient(180deg,rgba(255,255,255,`)
			b.WriteString(strconv.FormatFloat(tintAlpha, 'f', 3, 64))
			b.WriteString(`),rgba(255,255,255,`)
			b.WriteString(strconv.FormatFloat(tintAlpha/1.8, 'f', 3, 64))
			b.WriteString(`));box-shadow:inset 0 1px 0 rgba(255,255,255,.22),inset 0 -40px 80px rgba(255,255,255,.04);}</style>`)
		}
	}
	if options.BookmarkCategoryColor != "" || options.BookmarkItemColor != "" {
		b.WriteString(`<style>`)
		if options.BookmarkCategoryColor != "" {
			b.WriteString(`.bookmark-module .bookmark-group-container h3.bookmark-group-title,.bookmark-module .bookmark-subdir summary,.bookmark-module .bookmark-subdir summary::before{color:`)
			b.WriteString(options.BookmarkCategoryColor)
			b.WriteString(`;}`)
		}
		if options.BookmarkItemColor != "" {
			b.WriteString(`.bookmark-module .bookmark-group-container .bookmark-list a.bookmark,.bookmark-module .bookmark-group-container .bookmark-list a.bookmark span{color:`)
			b.WriteString(options.BookmarkItemColor)
			b.WriteString(`;}.bookmark-module .bookmark-group-container .bookmark-list a.bookmark img{color:`)
			b.WriteString(options.BookmarkItemColor)
			b.WriteString(`;}`)
		}
		b.WriteString(`</style>`)
	}
	return template.HTML(b.String())
}

func renderBackgroundHTML(assets background.Assets) template.HTML {
	if !assets.Enabled {
		return ""
	}

	previewSource := strings.TrimSpace(background.PreviewSource(assets))
	fullSource := template.HTMLEscapeString(assets.FullURL)
	var b strings.Builder
	b.WriteString(`<div class="page-background" aria-hidden="true">`)
	if previewSource != "" {
		b.WriteString(`<img class="page-background-preview" src="`)
		b.WriteString(template.HTMLEscapeString(previewSource))
		b.WriteString(`" alt="" loading="eager" fetchpriority="high" decoding="async">`)
	}
	b.WriteString(`<img class="page-background-full" src="`)
	b.WriteString(fullSource)
	b.WriteString(`" alt="" loading="eager" fetchpriority="high" decoding="async"></div>`)
	return template.HTML(b.String())
}

func cssURLValue(input string) string {
	input = strings.ReplaceAll(input, `\`, `\\`)
	input = strings.ReplaceAll(input, `'`, `\'`)
	input = strings.ReplaceAll(input, "\n", "")
	input = strings.ReplaceAll(input, "\r", "")
	return input
}

func pageAppearance(options model.Application, assets background.Assets) (template.CSS, string, error) {
	baseStyle, styleWarning, err := statuspage.RequireConfiguredBodyStyleForRender(options.Locale, "home")
	if err != nil {
		return "", "", err
	}
	base := string(baseStyle)
	previewSource := strings.TrimSpace(background.PreviewSource(assets))
	if previewSource == "" {
		return template.CSS(base), styleWarning, nil
	}
	opacity := options.BackgroundOpacity
	if opacity < 0 {
		opacity = 0
	}
	if opacity > 100 {
		opacity = 100
	}
	overlayAlpha := 1 - (float64(opacity) / 100)
	backgroundColor := extractBodyBackgroundColor(base)

	var b strings.Builder
	b.WriteString(base)
	if overlayAlpha > 0 {
		if backgroundColor == "" {
			backgroundColor = "rgba(26,26,26,1)"
		}
		b.WriteString(`background-image:linear-gradient(`)
		b.WriteString(rgbaOverlay(backgroundColor, overlayAlpha))
		b.WriteString(`,`)
		b.WriteString(rgbaOverlay(backgroundColor, overlayAlpha))
		b.WriteString(`),url('`)
	} else {
		b.WriteString(`background-image:url('`)
	}
	b.WriteString(cssURLValue(previewSource))
	b.WriteString(`');background-position:center center;background-repeat:no-repeat;background-size:cover;`)
	return template.CSS(b.String()), styleWarning, nil
}

func extractBodyBackgroundColor(base string) string {
	const prefix = "--color-background:"
	idx := strings.Index(base, prefix)
	if idx < 0 {
		return ""
	}
	start := idx + len(prefix)
	end := strings.Index(base[start:], ";")
	if end < 0 {
		return strings.TrimSpace(base[start:])
	}
	return strings.TrimSpace(base[start : start+end])
}

func rgbaOverlay(color string, alpha float64) string {
	alpha = clampFloat(alpha, 0, 1)
	color = strings.TrimSpace(color)
	if strings.HasPrefix(color, "#") {
		if r, g, b, ok := hexToRGB(color); ok {
			return fmt.Sprintf("rgba(%d,%d,%d,%.3f)", r, g, b, alpha)
		}
	}
	if strings.HasPrefix(strings.ToLower(color), "rgb(") {
		parts := strings.TrimSuffix(strings.TrimPrefix(color, "rgb("), ")")
		return "rgba(" + parts + "," + strconv.FormatFloat(alpha, 'f', 3, 64) + ")"
	}
	if strings.HasPrefix(strings.ToLower(color), "rgba(") {
		parts := strings.TrimSuffix(strings.TrimPrefix(color, "rgba("), ")")
		items := strings.Split(parts, ",")
		if len(items) >= 3 {
			return "rgba(" + strings.TrimSpace(items[0]) + "," + strings.TrimSpace(items[1]) + "," + strings.TrimSpace(items[2]) + "," + strconv.FormatFloat(alpha, 'f', 3, 64) + ")"
		}
	}
	return fmt.Sprintf("rgba(26,26,26,%.3f)", alpha)
}

func hexToRGB(color string) (int, int, int, bool) {
	hex := strings.TrimPrefix(strings.TrimSpace(color), "#")
	switch len(hex) {
	case 3:
		r, errR := strconv.ParseUint(strings.Repeat(string(hex[0]), 2), 16, 8)
		g, errG := strconv.ParseUint(strings.Repeat(string(hex[1]), 2), 16, 8)
		b, errB := strconv.ParseUint(strings.Repeat(string(hex[2]), 2), 16, 8)
		return int(r), int(g), int(b), errR == nil && errG == nil && errB == nil
	case 6, 8:
		r, errR := strconv.ParseUint(hex[0:2], 16, 8)
		g, errG := strconv.ParseUint(hex[2:4], 16, 8)
		b, errB := strconv.ParseUint(hex[4:6], 16, 8)
		return int(r), int(g), int(b), errR == nil && errG == nil && errB == nil
	default:
		return 0, 0, 0, false
	}
}

func clampFloat(value float64, min float64, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func appendAdaptiveColumnStyle(b *strings.Builder, maxColumns int) {
	if maxColumns < 1 {
		return
	}
	if maxColumns > 8 {
		maxColumns = 8
	}
	const (
		minColumnWidth    = 180
		appColumnGap      = 18
		bookmarkColumnGap = 18
		bookmarkMasonryAt = 560
	)
	b.WriteString(`<style>`)
	b.WriteString(`@media (min-width:1201px){#page-home.pageview .container{padding-left:clamp(40px,4vw,250px);padding-right:clamp(40px,4vw,250px);}}`)
	b.WriteString(fmt.Sprintf(`.apps-surface{display:grid;grid-template-columns:repeat(auto-fill,minmax(max(%dpx,calc((100%% - (%d - 1) * %dpx) / %d)),1fr));column-gap:%dpx;row-gap:0;align-items:start;}.apps-surface .app-container{float:none;width:auto;min-width:0;}.bookmark-module .bookmark-groups{display:grid;grid-template-columns:repeat(auto-fill,minmax(max(%dpx,calc((100%% - (%d - 1) * %dpx) / %d)),1fr));column-count:auto;column-gap:%dpx;gap:%dpx;align-items:start;}.bookmark-module .bookmark-group-container{break-inside:auto;display:block;width:auto;max-width:none;min-width:0;float:none;margin-bottom:0;vertical-align:top;align-self:start;}`,
		minColumnWidth, maxColumns, appColumnGap, maxColumns, appColumnGap,
		minColumnWidth, maxColumns, bookmarkColumnGap, maxColumns, bookmarkColumnGap, bookmarkColumnGap,
	))
	mobileColumns := 2
	if maxColumns < mobileColumns {
		mobileColumns = maxColumns
	}
	if mobileColumns < 1 {
		mobileColumns = 1
	}
	b.WriteString(fmt.Sprintf(`@media (max-width:%dpx){.bookmark-module .bookmark-groups{display:block;column-count:%d;column-gap:%dpx;}.bookmark-module .bookmark-group-container{break-inside:avoid;display:inline-block;width:100%%;max-width:none;min-width:0;float:none;margin-bottom:%dpx;vertical-align:top;}}`, bookmarkMasonryAt, mobileColumns, bookmarkColumnGap, bookmarkColumnGap))
	b.WriteString(`@media (max-width:340px){.bookmark-module .bookmark-groups{column-count:1;}}`)
	b.WriteString(`</style>`)
}

func RegisterRouting(e *echo.Echo) {
	e.GET(define.RegularPages.Home.Path, pageHome)
	e.POST(define.RegularPages.Home.Path, pageSearch)
	e.GET(define.RegularPages.Help.Path, renderHelp)
	if currentHomeRuntime().Visibility != "PRIVATE" {
		e.GET(define.RegularPages.Applications.Path, pageApplication)
		e.GET(define.RegularPages.Bookmarks.Path, pageBookmark)
	} else {
		e.GET(define.RegularPages.Applications.Path, pageApplication, auth.AuthRequired)
		e.GET(define.RegularPages.Bookmarks.Path, pageBookmark, auth.AuthRequired)
	}
}

func pageHome(c *echo.Context) error {
	return render(c, "")
}

func canViewPrivateItems(c *echo.Context) bool {
	if auth.IsLoginDisabled(c) {
		return true
	}
	return auth.ResolveLoginDisplayStateForView(c).ShowLoginInfo
}

func resolveHomeAssets(options model.Application) (background.Assets, error) {
	source := strings.TrimSpace(options.BackgroundImage)
	if source == "" {
		return background.Assets{}, nil
	}
	if strings.EqualFold(strings.TrimSpace(options.BackgroundImageMode), "upload") || strings.HasPrefix(source, "/user-assets/") {
		if _, _, err := background.FetchUploadedVariant("full"); err != nil {
			return background.Assets{}, fmt.Errorf("resolve uploaded background asset failed: %w", err)
		}
	}
	return background.ResolveAssets(options), nil
}

func renderHelp(c *echo.Context) error {
	options, err := data.GetAllSettingsOptions()
	if err != nil {
		statuspage.BindOptionsLoadError(c, err)
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, err.Error()))
	}
	statuspage.BindOptions(c, options)
	options, renderWarnings := statuspage.PrepareHomeOptionsForRender(options)
	now := time.Now()
	locale := options.Locale
	assets, err := resolveHomeAssets(options)
	if err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(locale, http.StatusInternalServerError, err.Error()))
	}
	pageStyle, styleWarning, err := pageAppearance(options, assets)
	if err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(locale, http.StatusInternalServerError, err.Error()))
	}
	if styleWarning != "" {
		renderWarnings = append(renderWarnings, styleWarning)
	}
	scriptNonce, err := maybeMakeScriptNonce(options.ShowDateTime || assets.Enabled || hasAsyncSiteIconRefresh(options))
	if err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(locale, http.StatusInternalServerError, err.Error()))
	}
	setCSPHeader(c, scriptNonce)
	bodyClassName := getBodyClassName(options)
	m := pool.GetTemplateMap()
	defer pool.PutTemplateMap(m)
	m["Locale"] = locale
	m["PageName"] = "Home"
	m["PageAppearance"] = pageStyle
	m["SettingPages"] = define.SettingPages
	m["DebugMode"] = currentHomeRuntime().DebugMode
	m["PageInlineStyle"] = define.GetPageInlineStyle()
	m["HeroDate"] = now.Format(i18n.DateFormat(locale))
	m["HeroTime"] = now.Format("15:04:05")
	m["HeroDay"] = i18n.Weekday(locale, now.Weekday())
	m["Greetings"] = i18n.T(locale, "page_help")
	m["BookmarksURI"] = define.RegularPages.Bookmarks.Path
	m["ApplicationsURI"] = define.RegularPages.Applications.Path
	m["SettingsURI"] = define.RegularPages.Settings.Path
	m["AppsTitle"] = resolveAppsTitle(options, locale)
	m["BookmarksTitle"] = resolveBookmarksTitle(options, locale)
	m["Applications"] = GenerateHelpTemplate(locale)
	m["SearchKeyword"] = template.HTML(buildSearchPlaceholder(options, locale))
	m["SearchHintLabel"] = buildSearchHintLabel(options, locale)
	m["SearchFormTarget"] = buildSearchFormTarget(options)
	m["SearchFormRel"] = buildSearchFormRel(options)
	m["HasKeyword"] = false
	m["ShowSearchComponent"] = options.ShowSearchComponent
	m["DisabledSearchAutoFocus"] = true
	m["OptionTitle"] = options.Title
	m["OptionSiteIcon"] = options.SiteIcon
	footer.BindTemplateData(m, options.Footer)
	m["OptionOpenAppNewTab"] = options.OpenAppNewTab
	m["OptionOpenBookmarkNewTab"] = options.OpenBookmarkNewTab
	m["OptionShowTitle"] = options.ShowTitle
	m["OptionShowDateTime"] = options.ShowDateTime
	m["OptionShowApps"] = true
	m["OptionShowBookmarks"] = false
	m["OptionHideSettingsButton"] = options.HideSettingsButton
	m["OptionHideHelpButton"] = options.HideHelpButton
	m["OptionHideWarningsButton"] = options.HideWarningsButton
	m["BodyClassName"] = template.HTMLAttr(bodyClassName)
	m["BackgroundAssets"] = assets
	m["HasBackgroundAssets"] = assets.Enabled
	m["BackgroundHTML"] = renderBackgroundHTML(assets)
	m["CustomHomeStyle"] = customHomeStyle(options, assets)
	m["RenderWarnings"] = renderWarnings
	m["HasRenderWarnings"] = len(renderWarnings) > 0
	m["ScriptNonce"] = scriptNonce
	m["InlineClockScript"] = template.JS(_inlineClockScript)
	m["InlineBackgroundLoaderScript"] = template.JS(_inlineBackgroundLoaderScript)
	m["InlineSiteIconRefreshScript"] = inlineSiteIconRefreshScript(options)
	return c.Render(http.StatusOK, "home.html", m)
}

func pageSearch(c *echo.Context) error {
	if err := statuspage.BindCurrentOptions(c); err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, err.Error()))
	}
	options, err := data.GetAllSettingsOptions()
	if err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, err.Error()))
	}
	var body struct {
		Search string `form:"search"`
	}
	if err := c.Bind(&body); err != nil {
		return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusBadRequest, "missing form data"))
	}
	search := strings.TrimSpace(body.Search)
	if len(search) > 50 {
		return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusBadRequest, "search query too long"))
	}
	if shouldUseExternalSearch(options) && search != "" {
		target, err := buildSearchEngineURL(options, search)
		if err != nil {
			return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, err.Error()))
		}
		return c.Redirect(http.StatusFound, target)
	}
	return render(c, search)
}

func getGreeting(greeting, locale string) string {
	defaultWord := i18n.T(locale, "greetings_placeholder")
	words := splitGreetingOptions(greeting)
	count := len(words)
	if count == 0 {
		return defaultWord
	}
	if count == 1 {
		return words[0]
	}
	if count != 4 {
		return words[mathrand.Intn(count)] //#nosec G404 -- non-crypto randomness is sufficient for rotating greetings
	}
	hour, _, _ := time.Now().Clock()
	if hour >= 5 && hour <= 10 {
		return words[0]
	}
	if hour >= 11 && hour <= 13 {
		return words[1]
	}
	if hour >= 14 && hour <= 18 {
		return words[2]
	}
	return words[3]
}

func splitGreetingOptions(greeting string) []string {
	parts := strings.Split(greeting, ";")
	result := make([]string, 0, len(parts))
	for _, item := range parts {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func pageBookmark(c *echo.Context) error {
	options, err := data.GetAllSettingsOptions()
	if err != nil {
		statuspage.BindOptionsLoadError(c, err)
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, err.Error()))
	}
	statuspage.BindOptions(c, options)
	options, renderWarnings := statuspage.PrepareHomeOptionsForRender(options)
	locale := options.Locale
	requestURL := fn.ParseRequestURLTo(c.Request())
	assets, err := resolveHomeAssets(options)
	if err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(locale, http.StatusInternalServerError, err.Error()))
	}
	pageStyle, styleWarning, err := pageAppearance(options, assets)
	if err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(locale, http.StatusInternalServerError, err.Error()))
	}
	if styleWarning != "" {
		renderWarnings = append(renderWarnings, styleWarning)
	}
	canViewPrivate := canViewPrivateItems(c)
	bookmarkModules, err := generateBookmarkModulesWithLocalAndURLErr("", &options, fn.RequestLooksLocalNetwork(c.Request()), &requestURL, canViewPrivate)
	if err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(locale, http.StatusInternalServerError, err.Error()))
	}
	renderWarnings = appendConfiguredIconWarningsForItems(locale, options.IconMode, false, nil, options.ShowBookmarks, bookmarkModules.items, renderWarnings)
	hasBookmarkDescriptions := options.ShowBookmarks && bookmarkModules.BookmarksHaveDescriptions
	scriptNonce, err := maybeMakeScriptNonce(assets.Enabled || hasAsyncSiteIconRefresh(options) || hasBookmarkDescriptions)
	if err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(locale, http.StatusInternalServerError, err.Error()))
	}
	setCSPHeader(c, scriptNonce)
	bodyClassName := getBodyClassName(options)
	m := pool.GetTemplateMap()
	defer pool.PutTemplateMap(m)
	m["Locale"] = locale
	m["DebugMode"] = currentHomeRuntime().DebugMode
	m["PageInlineStyle"] = define.GetPageInlineStyle()
	m["PageName"] = i18n.T(locale, "page_bookmarks")
	m["SubPage"] = true
	m["PageAppearance"] = pageStyle
	m["SettingPages"] = define.SettingPages
	m["BookmarksURI"] = define.RegularPages.Bookmarks.Path
	m["ApplicationsURI"] = define.RegularPages.Applications.Path
	m["SettingsURI"] = define.RegularPages.Settings.Path
	m["AppsTitle"] = resolveAppsTitle(options, locale)
	m["FavoritesTitle"] = resolveFavoritesTitle(options, locale)
	m["BookmarksTitle"] = resolveBookmarksTitle(options, locale)
	m["Bookmarks"] = bookmarkModules.Bookmarks
	m["Favorites"] = bookmarkModules.Favorites
	m["HasFavorites"] = bookmarkModules.HasFavorites
	m["BookmarksHaveDescriptions"] = bookmarkModules.BookmarksHaveDescriptions
	m["FavoritesHaveDescriptions"] = bookmarkModules.FavoritesHaveDescriptions
	m["HasBookmarkDescriptions"] = hasBookmarkDescriptions
	m["OptionTitle"] = options.Title
	m["OptionSiteIcon"] = options.SiteIcon
	footer.BindTemplateData(m, options.Footer)
	m["OptionOpenBookmarkNewTab"] = options.OpenBookmarkNewTab
	m["OptionShowBookmarks"] = options.ShowBookmarks
	m["OptionHideSettingsButton"] = options.HideSettingsButton
	m["OptionHideHelpButton"] = options.HideHelpButton
	m["OptionHideWarningsButton"] = options.HideWarningsButton
	m["BodyClassName"] = template.HTMLAttr(bodyClassName)
	m["BackgroundAssets"] = assets
	m["HasBackgroundAssets"] = assets.Enabled
	m["BackgroundHTML"] = renderBackgroundHTML(assets)
	m["CustomHomeStyle"] = customHomeStyle(options, assets)
	m["RenderWarnings"] = renderWarnings
	m["HasRenderWarnings"] = len(renderWarnings) > 0
	m["ScriptNonce"] = scriptNonce
	m["InlineBackgroundLoaderScript"] = template.JS(_inlineBackgroundLoaderScript)
	m["InlineSiteIconRefreshScript"] = inlineSiteIconRefreshScript(options)
	m["InlineBookmarkTooltipScript"] = inlineBookmarkTooltipScript(hasBookmarkDescriptions)
	return c.Render(http.StatusOK, "home.html", m)
}

func pageApplication(c *echo.Context) error {
	options, err := data.GetAllSettingsOptions()
	if err != nil {
		statuspage.BindOptionsLoadError(c, err)
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, err.Error()))
	}
	statuspage.BindOptions(c, options)
	options, renderWarnings := statuspage.PrepareHomeOptionsForRender(options)
	locale := options.Locale
	requestURL := fn.ParseRequestURLTo(c.Request())
	assets, err := resolveHomeAssets(options)
	if err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(locale, http.StatusInternalServerError, err.Error()))
	}
	pageStyle, styleWarning, err := pageAppearance(options, assets)
	if err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(locale, http.StatusInternalServerError, err.Error()))
	}
	if styleWarning != "" {
		renderWarnings = append(renderWarnings, styleWarning)
	}
	canViewPrivate := canViewPrivateItems(c)
	applications, err := generateApplicationProjectionWithLocalAndURLErr("", &options, fn.RequestLooksLocalNetwork(c.Request()), &requestURL, canViewPrivate)
	if err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(locale, http.StatusInternalServerError, err.Error()))
	}
	renderWarnings = appendConfiguredIconWarningsForItems(locale, options.IconMode, options.ShowApps, applications.items, false, nil, renderWarnings)
	scriptNonce, err := maybeMakeScriptNonce(assets.Enabled || hasAsyncSiteIconRefresh(options) || (options.ShowApps && applications.HasDirectories))
	if err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(locale, http.StatusInternalServerError, err.Error()))
	}
	setCSPHeader(c, scriptNonce)
	bodyClassName := getBodyClassName(options)
	m := pool.GetTemplateMap()
	defer pool.PutTemplateMap(m)
	m["Locale"] = locale
	m["DebugMode"] = currentHomeRuntime().DebugMode
	m["PageInlineStyle"] = define.GetPageInlineStyle()
	m["BookmarksURI"] = define.RegularPages.Bookmarks.Path
	m["ApplicationsURI"] = define.RegularPages.Applications.Path
	m["SettingsURI"] = define.RegularPages.Settings.Path
	m["AppsTitle"] = resolveAppsTitle(options, locale)
	m["BookmarksTitle"] = resolveBookmarksTitle(options, locale)
	m["Applications"] = applications.HTML
	m["ApplicationSubdirectoryModals"] = applications.Modals
	m["HasApplicationSubdirectories"] = applications.HasDirectories
	m["InlineApplicationSubdirectoryModalScript"] = inlineApplicationSubdirectoryModalScript(applications.HasDirectories)
	m["PageName"] = i18n.T(locale, "page_apps")
	m["SubPage"] = true
	m["PageAppearance"] = pageStyle
	m["OptionTitle"] = options.Title
	m["OptionSiteIcon"] = options.SiteIcon
	footer.BindTemplateData(m, options.Footer)
	m["OptionOpenAppNewTab"] = options.OpenAppNewTab
	m["OptionShowApps"] = options.ShowApps
	m["OptionHideSettingsButton"] = options.HideSettingsButton
	m["OptionHideHelpButton"] = options.HideHelpButton
	m["OptionHideWarningsButton"] = options.HideWarningsButton
	m["BodyClassName"] = template.HTMLAttr(bodyClassName)
	m["BackgroundAssets"] = assets
	m["HasBackgroundAssets"] = assets.Enabled
	m["BackgroundHTML"] = renderBackgroundHTML(assets)
	m["CustomHomeStyle"] = customHomeStyle(options, assets)
	m["RenderWarnings"] = renderWarnings
	m["HasRenderWarnings"] = len(renderWarnings) > 0
	m["ScriptNonce"] = scriptNonce
	m["InlineBackgroundLoaderScript"] = template.JS(_inlineBackgroundLoaderScript)
	m["InlineSiteIconRefreshScript"] = inlineSiteIconRefreshScript(options)
	return c.Render(http.StatusOK, "home.html", m)
}

func render(c *echo.Context, filter string) error {
	options, err := data.GetAllSettingsOptions()
	if err != nil {
		statuspage.BindOptionsLoadError(c, err)
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, err.Error()))
	}
	statuspage.BindOptions(c, options)
	options, renderWarnings := statuspage.PrepareHomeOptionsForRender(options)
	requestURL := fn.ParseRequestURLTo(c.Request())
	locale := options.Locale
	hasKeyword := false
	searchKeyword := buildSearchPlaceholder(options, locale)
	searchHintLabel := buildSearchHintLabel(options, locale)
	if filter != "" {
		searchKeyword = i18n.Tf(locale, "search_result", filter)
		hasKeyword = true
	}
	now := time.Now()
	assets, err := resolveHomeAssets(options)
	if err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(locale, http.StatusInternalServerError, err.Error()))
	}
	pageStyle, styleWarning, err := pageAppearance(options, assets)
	if err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(locale, http.StatusInternalServerError, err.Error()))
	}
	if styleWarning != "" {
		renderWarnings = append(renderWarnings, styleWarning)
	}
	preferLocal := fn.RequestLooksLocalNetwork(c.Request())
	canViewPrivate := canViewPrivateItems(c)
	applications, err := generateApplicationProjectionWithLocalAndURLErr(filter, &options, preferLocal, &requestURL, canViewPrivate)
	if err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(locale, http.StatusInternalServerError, err.Error()))
	}
	bookmarkModules, err := generateBookmarkModulesWithLocalAndURLErr(filter, &options, preferLocal, &requestURL, canViewPrivate)
	if err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(locale, http.StatusInternalServerError, err.Error()))
	}
	bookmarkWarningItems := bookmarkModules.items
	showBookmarkWarnings := options.ShowBookmarks
	if !showBookmarkWarnings && options.ShowFavorites {
		bookmarkWarningItems = bookmarkModules.favoriteItems
		showBookmarkWarnings = bookmarkModules.HasFavorites
	}
	renderWarnings = appendConfiguredIconWarningsForItems(locale, options.IconMode, options.ShowApps, applications.items, showBookmarkWarnings, bookmarkWarningItems, renderWarnings)
	scriptNonce, err := maybeMakeScriptNonce(options.ShowDateTime || assets.Enabled || hasAsyncSiteIconRefresh(options) || bookmarkModules.HasDescriptions || (options.ShowApps && applications.HasDirectories))
	if err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(locale, http.StatusInternalServerError, err.Error()))
	}
	setCSPHeader(c, scriptNonce)
	bodyClassName := getBodyClassName(options)
	m := pool.GetTemplateMap()
	defer pool.PutTemplateMap(m)
	m["Locale"] = locale
	m["PageName"] = "Home"
	m["PageAppearance"] = pageStyle
	m["SettingPages"] = define.SettingPages
	m["DebugMode"] = currentHomeRuntime().DebugMode
	m["PageInlineStyle"] = define.GetPageInlineStyle()
	m["HeroDate"] = now.Format(i18n.DateFormat(locale))
	m["HeroTime"] = now.Format("15:04:05")
	m["HeroDay"] = i18n.Weekday(locale, now.Weekday())
	m["Greetings"] = getGreeting(options.Greetings, locale)
	m["BookmarksURI"] = define.RegularPages.Bookmarks.Path
	m["ApplicationsURI"] = define.RegularPages.Applications.Path
	m["SettingsURI"] = define.RegularPages.Settings.Path
	m["AppsTitle"] = resolveAppsTitle(options, locale)
	m["FavoritesTitle"] = resolveFavoritesTitle(options, locale)
	m["BookmarksTitle"] = resolveBookmarksTitle(options, locale)
	m["Applications"] = applications.HTML
	m["ApplicationSubdirectoryModals"] = applications.Modals
	m["HasApplicationSubdirectories"] = applications.HasDirectories
	m["InlineApplicationSubdirectoryModalScript"] = inlineApplicationSubdirectoryModalScript(applications.HasDirectories)
	m["Bookmarks"] = bookmarkModules.Bookmarks
	m["Favorites"] = bookmarkModules.Favorites
	m["HasFavorites"] = bookmarkModules.HasFavorites
	m["BookmarksHaveDescriptions"] = bookmarkModules.BookmarksHaveDescriptions
	m["FavoritesHaveDescriptions"] = bookmarkModules.FavoritesHaveDescriptions
	m["HasBookmarkDescriptions"] = bookmarkModules.HasDescriptions
	m["SearchKeyword"] = template.HTML(searchKeyword)
	m["SearchHintLabel"] = searchHintLabel
	m["SearchFormTarget"] = buildSearchFormTarget(options)
	m["SearchFormRel"] = buildSearchFormRel(options)
	m["HasKeyword"] = hasKeyword
	m["ShowSearchComponent"] = options.ShowSearchComponent
	m["DisabledSearchAutoFocus"] = options.DisabledSearchAutoFocus
	m["OptionTitle"] = options.Title
	m["OptionSiteIcon"] = options.SiteIcon
	footer.BindTemplateData(m, options.Footer)
	m["OptionOpenAppNewTab"] = options.OpenAppNewTab
	m["OptionOpenBookmarkNewTab"] = options.OpenBookmarkNewTab
	m["OptionShowTitle"] = options.ShowTitle
	m["OptionShowDateTime"] = options.ShowDateTime
	m["OptionShowApps"] = options.ShowApps
	m["OptionShowFavorites"] = options.ShowFavorites && bookmarkModules.HasFavorites
	m["OptionShowBookmarks"] = options.ShowBookmarks
	m["OptionHideSettingsButton"] = options.HideSettingsButton
	m["OptionHideHelpButton"] = options.HideHelpButton
	m["OptionHideWarningsButton"] = options.HideWarningsButton
	m["BodyClassName"] = template.HTMLAttr(bodyClassName)
	m["BackgroundAssets"] = assets
	m["HasBackgroundAssets"] = assets.Enabled
	m["BackgroundHTML"] = renderBackgroundHTML(assets)
	m["CustomHomeStyle"] = customHomeStyle(options, assets)
	m["RenderWarnings"] = renderWarnings
	m["HasRenderWarnings"] = len(renderWarnings) > 0
	m["ScriptNonce"] = scriptNonce
	m["InlineClockScript"] = template.JS(_inlineClockScript)
	m["InlineBackgroundLoaderScript"] = template.JS(_inlineBackgroundLoaderScript)
	m["InlineSiteIconRefreshScript"] = inlineSiteIconRefreshScript(options)
	m["InlineBookmarkTooltipScript"] = inlineBookmarkTooltipScript(bookmarkModules.HasDescriptions)
	return c.Render(http.StatusOK, "home.html", m)
}

func shouldUseExternalSearch(options model.Application) bool {
	return strings.EqualFold(strings.TrimSpace(options.SearchMode), "engine")
}

func buildSearchPlaceholder(options model.Application, locale string) string {
	if !shouldUseExternalSearch(options) {
		return i18n.T(locale, "search_placeholder")
	}
	return i18n.Tf(locale, "search_engine_placeholder", searchEngineDisplayName(options, locale))
}

func buildSearchHintLabel(options model.Application, locale string) string {
	if !shouldUseExternalSearch(options) {
		return i18n.T(locale, "search_label")
	}
	return i18n.Tf(locale, searchEngineLabelKey(options), searchEngineDisplayName(options, locale))
}

func searchEngineDisplayName(options model.Application, locale string) string {
	switch strings.ToLower(strings.TrimSpace(options.SearchEngine)) {
	case "baidu":
		return "Baidu"
	case "bing":
		return "Bing"
	case "google":
		return "Google"
	case "duckduckgo":
		return "DuckDuckGo"
	case "custom":
		return i18n.T(locale, "search_engine_custom")
	default:
		return "Bing"
	}
}

func buildSearchEngineURL(options model.Application, keyword string) (string, error) {
	escaped := url.QueryEscape(keyword)
	templateValue := searchEngineTemplate(options)
	if !strings.Contains(templateValue, "%s") {
		return "", fmt.Errorf("custom search engine template must contain %%s placeholder")
	}
	return strings.ReplaceAll(templateValue, "%s", escaped), nil
}

func searchEngineTemplate(options model.Application) string {
	switch strings.ToLower(strings.TrimSpace(options.SearchEngine)) {
	case "baidu":
		return "https://www.baidu.com/s?wd=%s"
	case "bing":
		return "https://www.bing.com/search?q=%s"
	case "google":
		return "https://www.google.com/search?q=%s"
	case "duckduckgo":
		return "https://duckduckgo.com/?q=%s"
	case "custom":
		return strings.TrimSpace(options.SearchEngineCustomTemplate)
	default:
		return "https://www.bing.com/search?q=%s"
	}
}

func buildSearchFormTarget(options model.Application) string {
	if !shouldUseExternalSearch(options) {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(options.SearchEngineOpenMode), "new-tab") {
		return "_blank"
	}
	return ""
}

func buildSearchFormRel(options model.Application) string {
	if !shouldUseExternalSearch(options) {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(options.SearchEngineOpenMode), "new-tab") {
		return "noopener noreferrer"
	}
	return ""
}

func searchEngineLabelKey(options model.Application) string {
	if strings.EqualFold(strings.TrimSpace(options.SearchEngineOpenMode), "new-tab") {
		return "search_engine_label_new_tab"
	}
	return "search_engine_label"
}

func getBodyClassName(options model.Application) string {
	bodyClassName := ""
	if !options.KeepLetterCase {
		bodyClassName += "app-content-uppercase "
	}
	if options.GlassEffect == "frosted" || options.GlassEffect == "liquid" {
		bodyClassName += "glass-" + options.GlassEffect + " "
	}
	return bodyClassName
}

func resolveAppsTitle(options model.Application, locale string) string {
	if strings.TrimSpace(options.AppsTitle) != "" {
		return options.AppsTitle
	}
	return i18n.T(locale, "apps")
}

func resolveFavoritesTitle(options model.Application, locale string) string {
	if title := strings.TrimSpace(options.FavoritesTitle); title != "" {
		return title
	}
	return i18n.T(locale, "favorites")
}

func resolveBookmarksTitle(options model.Application, locale string) string {
	if strings.TrimSpace(options.BookmarksTitle) != "" {
		return options.BookmarksTitle
	}
	return i18n.T(locale, "bookmarks")
}

func getCSPValue(scriptNonce string) string {
	if scriptNonce == "" {
		return _cspScriptNone + _cspValue
	}
	return "script-src 'nonce-" + scriptNonce + "'; " + _cspValue
}

func maybeMakeScriptNonce(enabled bool) (string, error) {
	if !enabled {
		return "", nil
	}
	buf := make([]byte, 16)
	if _, err := cryptoRandRead(buf); err != nil {
		return "", fmt.Errorf("generate script nonce failed: %w", err)
	} else {
		return base64.RawStdEncoding.EncodeToString(buf), nil
	}
}

func inlineSiteIconRefreshScript(options model.Application) template.JS {
	if !hasAsyncSiteIconRefresh(options) {
		return ""
	}
	return template.JS(_inlineSiteIconRefreshScript)
}

func inlineBookmarkTooltipScript(enabled bool) template.JS {
	if !enabled {
		return ""
	}
	return template.JS(_inlineBookmarkTooltipScript)
}

func inlineApplicationSubdirectoryModalScript(enabled bool) template.JS {
	if !enabled {
		return ""
	}
	return template.JS(_inlineApplicationSubdirectoryModalScript)
}

func hasAsyncSiteIconRefresh(options model.Application) bool {
	return strings.ToUpper(strings.TrimSpace(options.IconMode)) != define.IconModeHidden
}
