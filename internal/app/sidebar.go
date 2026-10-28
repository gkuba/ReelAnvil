package app

import (
	"bytes"
	"html/template"
)

const sidebarTemplateSource = `<aside id="app-sidebar">
<div class="rail-head"><a class="brand" href="/" aria-label="ReelAnvil home"><img class="brand-mark" src="{{.IconURL}}" alt=""><span>REEL<span>ANVIL</span></span></a><button type="button" id="rail-toggle" aria-label="Collapse sidebar" title="Toggle sidebar"><svg class="rail-icon" viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M9 4v16"/></svg></button></div>
<nav aria-label="Main navigation">
<a href="/" title="Overview" {{if eq .Section "overview"}}class="active" aria-current="page"{{end}}><svg class="rail-icon" viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="3" width="8" height="8" rx="1"/><rect x="13" y="3" width="8" height="5" rx="1"/><rect x="13" y="10" width="8" height="11" rx="1"/><rect x="3" y="13" width="8" height="8" rx="1"/></svg><span>Overview</span></a>
<a href="/campaigns" title="Campaigns" {{if eq .Section "campaigns"}}class="active" aria-current="page"{{end}}><svg class="rail-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M3 10v4a2 2 0 0 0 2 2h2l3 5h3l-2-6 9 3V6l-9 3H5a2 2 0 0 0-2 1z"/><path d="M19 9.5a4 4 0 0 1 0 5"/></svg><span>Campaigns</span></a>
<a href="/jobs" title="Jobs" {{if eq .Section "jobs"}}class="active" aria-current="page"{{end}}><svg class="rail-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M9 6h11M9 12h11M9 18h11"/><path d="m3 6 1.5 1.5L7 5M3 12l1.5 1.5L7 11M3 18l1.5 1.5L7 17"/></svg><span>Jobs</span></a>
<div class="nav-divider" role="separator"></div>
<div class="nav-group" data-group="media"><button type="button" class="nav-group-toggle" aria-expanded="{{if or (eq .Section "media") (eq .Section "review")}}true{{else}}false{{end}}" title="Media"><svg class="rail-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M4 5h16v14H4zM4 9h16M8 5l2 4m2-4 2 4m2-4 2 4"/><path d="m10 12 5 2.5-5 2.5z"/></svg><span>Media</span><svg class="rail-icon chevron" viewBox="0 0 24 24" aria-hidden="true"><path d="m9 18 6-6-6-6"/></svg></button><div class="nav-children"><a href="/media" title="Media Library" {{if eq .Section "media"}}class="active" aria-current="page"{{end}}><svg class="rail-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M5 4h13a2 2 0 0 1 2 2v14H7a2 2 0 0 1-2-2z"/><path d="M5 17H3V6a2 2 0 0 1 2-2m4 4h7m-7 4h7m-7 4h5"/></svg><span>Media Library</span></a><a href="/review" title="Review" {{if eq .Section "review"}}class="active" aria-current="page"{{end}}><svg class="rail-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z"/><circle cx="12" cy="12" r="3"/><path d="m10.5 12 1 1 2-2"/></svg><span>Review</span></a></div></div>
<div class="nav-divider" role="separator"></div>
<a href="/diagnostics" title="Diagnostics" {{if eq .Section "diagnostics"}}class="active" aria-current="page"{{end}}><svg class="rail-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M3 12h4l3-8 4 16 3-8h4"/></svg><span>Diagnostics</span></a>
<div class="nav-group" data-group="settings"><button type="button" class="nav-group-toggle" aria-expanded="{{if or (eq .Section "settings") (eq .Section "themes") (eq .Section "connectors") (eq .Section "storage") (eq .Section "services")}}true{{else}}false{{end}}" title="Settings"><svg class="rail-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8z"/><path d="m19.4 15 .1.1 1.3 1-1.5 2.6-1.6-.6-.2.1-1.4.8-.3 1.7h-3l-.3-1.7-1.4-.8-.2-.1-1.6.6-1.5-2.6 1.3-1 .1-.2v-1.6l-.1-.2-1.3-1L8.8 9l1.6.6.2-.1 1.4-.8.3-1.7h3l.3 1.7 1.4.8.2.1 1.6-.6 1.5 2.6-1.3 1-.1.2z" transform="translate(-1 -1)"/></svg><span>Settings</span><svg class="rail-icon chevron" viewBox="0 0 24 24" aria-hidden="true"><path d="m9 18 6-6-6-6"/></svg></button><div class="nav-children"><a href="/settings" title="General" {{if eq .Section "settings"}}class="active" aria-current="page"{{end}}><svg class="rail-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M4 6h16M4 12h16M4 18h16"/><circle cx="9" cy="6" r="2"/><circle cx="15" cy="12" r="2"/></svg><span>General</span></a><a href="/settings/themes" title="Themes" {{if eq .Section "themes"}}class="active" aria-current="page"{{end}}><svg class="rail-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3a9 9 0 1 0 0 18h2a2 2 0 0 0 1-3.7 2 2 0 0 1 1-3.7h2a3 3 0 0 0 3-3A9 9 0 0 0 12 3z"/><circle cx="7" cy="11" r="1"/><circle cx="12" cy="7" r="1"/></svg><span>Themes</span></a><a href="/settings/connectors" title="Connectors" {{if eq .Section "connectors"}}class="active" aria-current="page"{{end}}><svg class="rail-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M8 3v5m8-5v5M5 8h14v3a7 7 0 0 1-14 0V8zm7 10v4"/></svg><span>Connectors</span></a><a href="/settings/storage" title="Paths &amp; Storage" {{if eq .Section "storage"}}class="active" aria-current="page"{{end}}><svg class="rail-icon" viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="5" width="18" height="14" rx="2"/><path d="M3 12h18"/><circle cx="17" cy="16" r="1"/></svg><span>Paths &amp; Storage</span></a><a href="/services" title="Services" {{if eq .Section "services"}}class="active" aria-current="page"{{end}}><svg class="rail-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3v2m0 14v2M3 12h2m14 0h2M5.6 5.6 7 7m10 10 1.4 1.4M18.4 5.6 17 7M7 17l-1.4 1.4"/><circle cx="12" cy="12" r="4"/></svg><span>Services</span></a></div></div>
</nav><div class="rail-flyout" id="rail-flyout" aria-label="Section navigation"></div></aside>`

var sidebarTemplate = template.Must(template.New("sidebar").Parse(sidebarTemplateSource))

func renderSidebar(section, icon string) template.HTML {
	var out bytes.Buffer
	if err := sidebarTemplate.Execute(&out, struct{ Section, IconURL string }{section, icon}); err != nil {
		panic(err)
	}
	return template.HTML(out.String())
}

const sidebarCSS = `
.icon-picker{border:0;margin:18px 0 0;padding:0}.icon-picker legend{color:var(--text);font-weight:700}.icon-picker p{margin:3px 0}
.shell{grid-template-columns:248px minmax(0,1fr)}.shell.rail-compact{grid-template-columns:64px minmax(0,1fr)}
#app-sidebar{position:relative;min-width:0;z-index:20;padding:16px 10px;background:var(--sidebar);border-right:1px solid var(--line)}
#app-sidebar .rail-head{display:flex;align-items:center;justify-content:space-between;gap:8px;min-height:40px;margin:0 0 18px}
#app-sidebar .brand{display:flex;align-items:center;gap:9px;min-width:0;padding:0;text-decoration:none;color:var(--text);font-size:18px;font-weight:800;letter-spacing:.4px;white-space:nowrap}
#app-sidebar .brand-mark{width:32px;height:32px;border-radius:7px;flex:none}
#app-sidebar .brand>span>span{color:var(--accent)}
#app-sidebar #rail-toggle{display:grid;place-items:center;width:34px;height:34px;padding:6px;background:transparent;color:var(--muted);border:1px solid transparent;flex:none}
#app-sidebar #rail-toggle:hover{background:color-mix(in srgb,var(--accent) 13%,var(--sidebar));border-color:var(--line);color:var(--text)}
#app-sidebar nav{display:flex;flex-direction:column;gap:3px;margin:0;padding:0}
#app-sidebar nav a,#app-sidebar .nav-group-toggle{display:flex;align-items:center;gap:11px;width:100%;min-height:40px;margin:0;padding:8px 10px;border:0;border-radius:9px;background:transparent;color:var(--muted);font:inherit;text-align:left;text-decoration:none;cursor:pointer}
#app-sidebar nav a:hover,#app-sidebar .nav-group-toggle:hover{background:color-mix(in srgb,var(--accent) 13%,var(--sidebar));color:var(--text)}
#app-sidebar nav a.active,#app-sidebar .nav-group-toggle.group-active{background:color-mix(in srgb,var(--accent) 23%,var(--sidebar));border-left:3px solid var(--accent);padding-left:7px;color:var(--text)}
#app-sidebar nav a.active .rail-icon,#app-sidebar .nav-group-toggle.group-active .rail-icon{color:var(--accent)}
#app-sidebar .rail-icon{width:19px;height:19px;flex:none;fill:none;stroke:currentColor;stroke-width:1.8;stroke-linecap:round;stroke-linejoin:round}
#app-sidebar nav a span,#app-sidebar .nav-group-toggle span{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
#app-sidebar .nav-group{width:100%}
#app-sidebar .nav-group-toggle{justify-content:flex-start;font-size:14px;font-weight:550}
#app-sidebar .nav-group-toggle .chevron{margin-left:auto;transition:transform .15s ease}
#app-sidebar .nav-group.open .nav-group-toggle .chevron{transform:rotate(90deg)}
#app-sidebar .nav-children{display:none;padding:3px 0 5px 10px}
#app-sidebar .nav-group.open .nav-children{display:block}
#app-sidebar .nav-children a{min-height:36px;font-size:13px}
#app-sidebar .nav-divider{height:1px;margin:8px 5px;background:color-mix(in srgb,var(--line) 70%,transparent)}
#app-sidebar .rail-flyout{display:none;position:absolute;z-index:30;top:105px;left:calc(100% + 8px);min-width:190px;padding:8px;border:1px solid var(--line);border-radius:11px;background:var(--surface);box-shadow:0 12px 30px #0007}
#app-sidebar .rail-flyout.visible{display:flex;flex-direction:column;gap:3px}
#app-sidebar .rail-flyout .flyout-title{padding:8px 10px;color:var(--muted);font-size:11px;font-weight:700;text-transform:uppercase;letter-spacing:1px}
#app-sidebar .rail-flyout a{display:flex;align-items:center;gap:9px;padding:8px 10px;color:var(--text);text-decoration:none;border-radius:7px}
#app-sidebar .rail-flyout a:hover,#app-sidebar .rail-flyout a:focus-visible{background:color-mix(in srgb,var(--accent) 16%,var(--surface))}
#app-sidebar .rail-flyout a.active{border-left:3px solid var(--accent);padding-left:7px}
.rail-compact #app-sidebar .brand>span,.rail-compact #app-sidebar nav a span,.rail-compact #app-sidebar .nav-group-toggle span,.rail-compact #app-sidebar .nav-group-toggle .chevron{display:none}
.rail-compact #app-sidebar .rail-head{justify-content:center;flex-wrap:wrap}.rail-compact #app-sidebar .brand{justify-content:center}.rail-compact #app-sidebar nav a,.rail-compact #app-sidebar .nav-group-toggle{justify-content:center;padding:9px 6px}
.rail-compact #app-sidebar .nav-group.open .nav-children{display:none}
@media(max-width:650px){.shell,.shell.rail-compact{grid-template-columns:minmax(0,1fr)}#app-sidebar{padding:8px 12px;border-right:0;border-bottom:1px solid var(--line)}#app-sidebar .rail-head{justify-content:flex-start;margin-bottom:7px}#app-sidebar nav{flex-direction:row;flex-wrap:wrap;gap:3px}#app-sidebar nav a,#app-sidebar .nav-group-toggle{width:auto;min-height:36px;padding:6px 9px;font-size:12px}#app-sidebar .nav-group-toggle .chevron{width:14px;height:14px}#app-sidebar .nav-children{padding-left:9px}#app-sidebar .nav-divider{width:1px;height:23px;margin:6px 4px}#app-sidebar .rail-flyout{top:100%;left:10px}.rail-compact #app-sidebar .brand>span,.rail-compact #app-sidebar nav a span,.rail-compact #app-sidebar .nav-group-toggle span,.rail-compact #app-sidebar .nav-group-toggle .chevron{display:inline}.rail-compact #app-sidebar .rail-head{justify-content:flex-start}.rail-compact #app-sidebar nav a,.rail-compact #app-sidebar .nav-group-toggle{justify-content:flex-start;padding:6px 9px}}
`

const sidebarScript = `(function(){const shell=document.querySelector('.shell'),sidebar=document.getElementById('app-sidebar'),toggle=document.getElementById('rail-toggle'),flyout=document.getElementById('rail-flyout');if(!shell||!sidebar||!toggle||!flyout)return;let compact=false;try{compact=localStorage.getItem('reelanvil-rail')==='compact'}catch(e){}function hideFlyout(){flyout.classList.remove('visible');flyout.replaceChildren()}function setCompact(v){compact=v;shell.classList.toggle('rail-compact',v);toggle.setAttribute('aria-label',v?'Expand sidebar':'Collapse sidebar');toggle.setAttribute('aria-expanded',String(!v));hideFlyout();try{localStorage.setItem('reelanvil-rail',v?'compact':'expanded')}catch(e){}}let groups=[...sidebar.querySelectorAll('.nav-group')];groups.forEach(g=>{const button=g.querySelector('.nav-group-toggle'),active=!!g.querySelector('a.active');g.classList.toggle('open',active);button.setAttribute('aria-expanded',String(active));button.classList.toggle('group-active',active);button.addEventListener('click',()=>{if(compact){if(flyout.classList.contains('visible')&&flyout.dataset.group===g.dataset.group){hideFlyout();return}flyout.dataset.group=g.dataset.group;flyout.replaceChildren();const title=document.createElement('div');title.className='flyout-title';title.textContent=button.querySelector('span').textContent;flyout.append(title);g.querySelectorAll('.nav-children a').forEach(a=>flyout.append(a.cloneNode(true)));flyout.classList.add('visible')}else{g.classList.toggle('open');button.setAttribute('aria-expanded',String(g.classList.contains('open')))}})});setCompact(compact);toggle.addEventListener('click',()=>setCompact(!compact));sidebar.addEventListener('keydown',e=>{if(e.key==='Escape')hideFlyout()});document.addEventListener('click',e=>{if(!sidebar.contains(e.target))hideFlyout()})})();`
