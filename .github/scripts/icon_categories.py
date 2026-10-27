"""Reviewed taxonomy for reproducible library maintenance, with no dependencies."""

import re
import json
from pathlib import Path


def normalized(value):
    return re.sub(r"[^a-z0-9]", "", value.lower())


UPSTREAM_CATEGORIES = {
    "系统工具": "Operating-Systems Linux-Distributions Home-Automation Smart-Home Password-Managers automation utility Software infrastructure",
    "开发编程": "Development Developer-Tools Programming-Languages Version-Control-Systems DevOps Databases database Containerization-&-Orchestration",
    "AI": "AI-&-LLM-Platforms AI Artificial-Intelligence",
    "网络工具": "Network Networking-Tools Monitoring-Tools monitoring Security networking VPN",
    "影音娱乐": "Media Media-Servers Music Music-Streaming Video-Streaming Streaming Video Entertainment Anime",
    "浏览器": "Web-Browsers",
    "办公效率": "Office-Suites Organization Note-taking-Apps productivity collaboration",
    "社交通讯": "Communication Social-Media Social Email-Providers Community",
    "网盘存储": "File-Sharing-&-Sync File Storage File-Management",
    "云服务": "Cloud Cloud-Computing Server-Panels self-hosted",
    "设备硬件": "Hardware",
    "金融支付": "Finance Finance-&-Banking",
    "游戏": "Gaming Gaming-Platforms",
    "电商购物": "E-commerce-Platforms E-Commerce",
    "教育科研": "Education",
    "新闻资讯": "News Technology-News",
    "搜索导航": "Search-Engines Search Information",
    "出行地图": "Travel Logistics Location",
    "设计素材": "Design Graphics-Editors",
    "生活服务": "Health Personal Animal",
    "阅读": "Wiki",
}
LABEL_CATEGORY = {normalized(label): category for category, labels in UPSTREAM_CATEGORIES.items() for label in labels.split()}
GENERIC_LABELS = {"tools", "utility", "automation", "software", "selfhosted", "infrastructure"}

# Exact product identifiers reviewed independently of empty/generic upstream
# labels. More specific assignments later in this mapping take precedence.
REVIEWED = {
    "金融支付": """actual-budget balanohidrico cashpilot ctrader declarenta elster erste erste-george finanzen-zero freshbooks ghostfolio helium-token invoiceplane japanese-tax-filing lnbits maybe monarch-money moneymatter oddinpay pepperstone proton-wallet receipt-wrangler richy rocket-money sparkasse tether wise-moneytransfer wownero xmrig ynab""",
    "开发编程": """actian agent-of-empires agentgateway anaconda apache-airflow apache-answer apache-iceberg apache-jena apache-solr apache-subversion apache-superset apache-tomcat apollo appium appwrite ara-records-ansible arduino artifacthub artifactory asciinema astral awx backstage baserow bat-file browserless budibase c caddy charcoal chipsound cilium cinc clickhouse cloud-native-pg cloudbeaver cms cockpit-cms coder concourse configarr confluent copyq couchdb cpp crossplane crowdin css csv ctfreak cups cypress dagster dart databricks datagrip dbgate ddev dencode dependency-track directus docassemble docsify docusaurus drupal eleventy etcd exe express fastapi flux-cd flux-operator fortran framer fulcio gatsby gitbook gitbucket gocd go golang goldilocks gsap harbor harvester hashicorp-packer hashicorp-waypoint haskell hasura helix helm hexedit homebrew hono html http-403-forbidden http-404-not-found http-418-im-a-teapot http-500-internal-server-error http-503-service-unavailable i-am-programmer influxdb istio jaeger jfrog json julia jumpserver jupyter jwt-io k3s kafka keda kinto kirby kong kotlin kubetail lakefs libredb-studio libretranslate lightpanda linuxserver-io lua maltego md medama mediawiki mermaid metabase micro mkdocs mongo-express mongodb neo4j neovim nocodb nuxt-js ocaml omnidb opa-ui open-regex open-source-initiative opensearch paper payload perl perlite phorge pi-coding-agent pkl plantuml pls-development-tools pocketbase portabase portus postman powershell powershell-file pypi qdrant qlik qlik2 questdb raftman rekor reloader replit requestly rstudio rundeck saltcorn scala selenium semaphore sentry serverspec shell-file shiori shlink slice slidev sofka sonarqube sourcehut sphinx sql sqlite sqlitebrowser squarespace squidex stacks starlight storm stormkit strapi sublime-merge surrealdb taiga taskwarrior teamcity technical-ethics telegraf tempo thanos theia toolhive tooljet tricentis-tosca tyepcho typemill typesense typo3 undb vector wakapi webflow webhook webhookd webkit wikijs wolframalpha yourls""",
    "AI": """anything-llm black-forest-labs boltnew chatplaygroundai claude-ai comfyui compreface dall-e diffusionbee docling eleven-labs eliza-os fromkai hermes hermes-agent higgsfield infomaniak-euria invoke-ai kaboai kimi-ai litlellm localai lovable lynxprompt mem0 mila open-notebook paperless-gpt poolside-ai pytorch runpod scriberr secureai-tools tensorflow tekstobot upscayl vllm whishper wispr-flow z-ai zenmux""",
    "网络工具": """8311 9router acars-hub acme adguard-home-sync airvpn akvorado alertmanager aliasvault altcha anydesk arkkvm authman autenticao-gov avg beszel bigfix bindplane bitly boundary breachlab burpsuite buypass cert-manager cert-warden certshell changedetection check-point checkmate checkmk citrix cloudflared connectwise crowdsec crowdsec-web-ui crowdstrike datadog ddclient ddns-updater defguard dns-private-resolver dns-zone dnsimple docusnap doozle dozzle dtop dreeve elastic-beats entra-id expressvpn fast-com firewalla flaresolverr fleetdm flexera fmd forgegate fortinet freeipa freshping frp-manager genua glances gluetun golink graphite graylog graywolf haproxy hashicorp-boundary headplane headscale healthchecks heylogin hudu i2p i2pd icinga infoblox itop ivanti jetkvm kanidm kaseya keepass keycloak keyoxide keyrack lastpass lets-encrypt level-io librenms librespeed linkace linkding loki meraki meshping meshtastic multifactor-authenticator mullvad mullvad-vpn n-able nagios name-silo nebula netbox netmap network-ups-tools networking-toolbox ngrok nsg nsclient ntfy nut oauth2 oneuptime ookla-speedtest openbao openscap openspeedtest openziti otp packetfence paessler-prtg palo-alto passbolt password-depot patchmon pdq peanut phoneinfoga pia pikvm pingdom pocket-id prometheus-node-exporter prtg pulseway pushover rapid7 recommander restreamer root-me router rport rsa scanopy scrutiny searx searxng self-host senstar shellhub shellngn silverfort simplelogin sing-box solarwinds sophos spamassassin spiceworks splashtop splunk sshwifty statuspage stb-proxy sub-store surfshark sysreptor tacticalrmm tailscale teleport thehive tianji timesketch traefik-home tryhackme tsd-proxy twingate unbound unimus unstoppabledomains untangle upsnap vaultwake vaultwarden vera-crypt virustotal voidauth vn-stat voip-ms warpgate whats-up-docker wudup xpipe xray yacd-blue yopass zoraxy""",
    "影音娱乐": """a-mule airdc ableton airsonic ammds angel-studios arrpulse auditorr auracast aurral autobrr av1 avif azuracast barrage bazarr beetstreamnext blu-ray blu-ray-3d byparr castopod cleanuperr configarr crosswatch denon digitelstudios dlna easyepg-lite endel enterr falcon-player feishin findroid fladder floatplane flood gonic get-iplayer google-podcasts google-tv hbo hdhomerun healarr hifiberry hyperpipe immich-frame iplayer jackett jellybox jellyfin-vue jellyseerr jellywatch kapowarr kick kinopub kitana krokiet kyoo libation librariarr lidarr liremdb lunasea lyrion m3u-editor magicinfo marreta mediux medusa memories midarr mlb-tv mousetrap musicbrainz-picard mylar-3 navidrome neko neutarr newshosting nikku nodecast-tv notifiarr nrk-logo nzbdav nzbhydra2 odysee odysseus ombi oscarr owncast panelarr paramount-plus peacock pepperbox-tv photonix photostructure picsur picture pigallery2 pigeonpod piped piwigo plexrequests pocket-casts podfetch podify polaris premiumize pyload qobuz qui radarr-dv rdt-client readarr recyclarr remuxcode retriever revanced-manager riverside-fm romm roon rss-bridge rss-translator rundfunkarr rygel samsung-tv-plus sat-ip scrob seerr servarr shinkro shoko shoko-server shokoanime sickbeard simkl sinusbot slskd snapcast sonarr-dv sonos sponsorblock sportarr subatic suwayomi svt-play swingmusic symfonium teamarr teamspeak telegram-archive the-pirate-bay the-proxy-bay tidal tmdb tpdb trailarr tubesync tuliprox tvdb tvheadend tynimedia video videogametrackarr vidzy viseron voilib vuetorrent waipu-tv watcharr weavestream webtorrent whisparr yamtrack your-spotify youtube-dl youtube-kids youtube-studio yt-dlp ytdl-material yts yukkuri-movie-maker-4""",
    "浏览器": """browsh falkon-breeze firefox-developer-edition firefox-lite firefox-nightly geckoview lynx mullvad-browser netsurf opera-touch palemoon qutebrowser safari zen-browser""",
    "办公效率": """aac-infotray abacus-research aboard act affine anaplan anytype apache-openoffice appflowy applire atlassian-opsgenie atlassian-trello basecamp bonfire brick-tracker cairn cal-com captivo collabora-online convertio cozy data-studio diagrams-net dillinger docuform documenso docuseal docusign docuware doxis donetick dumbpad enclosed eu-calendar eu-docs eu-presentation eu-spreadsheet everhour eway-crm foxit fruux goodnotes gotomaxx grist habitica haptic hcl-notes herdr heyform hivedav hoarder icewarp-office infor-xa kanboard kimai kitchenowl koillection leantime libreoffice limesurvey listmonk many-notes matrix42 mautic microsoft-365-admin-center microsoft-dataverse microsoft-project microsoft-visio milinote missionheroz mobioffice monica netsuite neonlink nextcloud-calendar nextcloud-contacts nextcloud-cookbook nextcloud-deck nextcloud-notes nextcloud-tables nextcloud-tasks nextcloud-timemanager notion-calendar novapdf nuclino odoo office-eu onlyoffice open-resume openoffice oott outline paperless paperless-ngx papermark papercut part-db partkeepr pastebin pdf pdf24 pdfding pdfforge peoplefluent pepperminty-wiki plane planka plume polarisoffice powerbi privatebin proalpha prolific proton-sheets rallly reactive-resume root-app salesforce satchelone seatable shaarli shortlistapp signaturepdf silae simpli sinf softmakerfreeoffice softmakeroffice solidtime stackfield stirling-pdf sunsama tabula tangerine-ui taskcafe teamtailor text-document thinkcell tiddlywiki timetagger toggl tolgee topdesk touitomamout trackeep twake-mail webtrees wedcm.com wekan wikidocs wiki-go wikidata wikimedia woorigi writefreely zendesk zoho zoho-desk""",
    "社交通讯": """3cx akkoma at-protocol broadcastchannel cheogram discourse dovecot docker-mailserver etesync eu-mail eu-talk flarum fluffychat fluxer franz freepbx friendica gboard gigaset gotosocial grandstream help-scout humhub icewarp infomaniak-kmeet invisioncommunity issabel-pbx jio jitsi-meet kbin lemmy libreddit littlelink-custom local-content-share mailbox mailchimp mailfence mailgun mail-in-a-box matrix-synapse mbin misskey-io mitra mixpost mobilizon nextcloud-social nitter postal postfix postiz proton-meet purelymail rainloop redlib rimgo sendinblue sogo sphinx-relay stalwart-mail-server stoat synapse team-viewer thelounge tuta-calendar tuta-mail zohomail""",
    "网盘存储": """acronis alist aomei borg borg-ui borgmatic binner cerberus-ftp-server copyparty cyberduck databasus droppy erugo eu-drive filebrowser filebrowser-quantum filecloud filegator filen files files-community filescom firefox-send freedombox freenas garage gose icloud lancache mail-archiver mediafire nextcloud-files nextcloud-ncdownloader pcloud proxmox-backup-server pure-storage pydio quickshare replicaro rockstor rustfs send sharewood sharry snapdrop snowshare storj sylve truenas-core truenas-scale versitygw voltaserve xbackbone xbrowsersync xigmanas xpenology zip-archive""",
    "云服务": """aks-automatic apache-cloudstack apiscp app-service application-gateway-container application-gateways aws-dynamodb aws-dynamodb-logo aws-ecs bunny caprover cloudpanel contabo cpanel deployarr directadmin docker-engine docker-moby dockge dockhand-svg dream-host emeraldhost enhance fly-io gridscale google-cloud-platform google-compute-engine hestia hotio ionos ispconfig latitudesh nutanix openstack ovirt parallels pikapods portainer-be proxcenter puter railway rancher runonflux runson strato stormkit tos ubports umbrel velero virtualmin vmware vmware-esxi vmware-nsx vmware-workstation xcloud xcp-ng xen xen-orchestra zomro""",
    "系统工具": """admin-by-request ai-on-the-edge-device alacritty armbian aroz-os bab-technologie balena-etcher baramundi bazecor btop calyxos casaos catppuccin chocolate cockpit coreos cosmic crafty-controller cronmaster dtop dumbassets dynacat e-os eisfair elysian endless euquista evcc f-droid fhem fit-dashboard flatcar flathub flatpak fluidd freedom fronius gnu-guix gnustep glance google-find google-password-manager grocy haiku hammond heimdall homebox homehub homelable hoobs huginn hydra hypermind iobroker iode-os jeedom jtu-me klipper krusader lab-dash lact lancommander lineage mafl mainsail maker-world manyfold matter mcaffee micro-off murena myheats netatmo netboot netbootxyz nextcloudpi niri novnc oh-my-posh olivetin open-central-control-unit openuem origin pelican-panel phoscon printstash printventory postmarketos prozilla-os prunemate prusa-research pterodactyl pufferpanel qd-today qemu rackpeek recalbox scriptrunner shelly solaar solus specifically-clementines spinsense supla talos tasmocompiler tasmota telekom-magenta-smarthome thingsboard treesize tuya-smart-inc uefi umbrel us valetudo victron-energy volkszaehler voron wayland wind-river windirstat wiz wolfi z-wave-js-ui zigbee zigbee2mqtt""",
    "设备硬件": """abus-group airtel alcatel-lucent-enterprise arrow-electronics asus-full asustor at-t avigilon avm-fritzbox avm-powerline avm-repeater bang-and-olufsen barco be-quiet bluetooth broadcom brocade dcmi dentsply-sirona draytek dunkermotoren durr-dental dymo elo-touch-solutions espressif fairphone forte garmin google-fi google-wifi hpe identiv iliadbox-svg itad ixon keenetic konica-minolta konica-minolta-symbol kpn lancom logitech-legacy mercusys mobotix mouser msi netapp netgear omada orange orangedental p1ib poly repeater-1200ax2 ricoh sargent-greenleaf sew-eurodrive siglent snapmaker symmetricom telekom telmex tenda terramaster thread toshiba ubiquiti ubiquiti-unifi ugreen verizon vertiv videojet vodafone volla zyxel-communications zyxel-networks""",
    "设计素材": """adobe-indesign autodesk-maya awwesome behance blue-check-mark br cc cd cura dashboard-icons dia draw-io font-awesome-icons fontawesome freecad google-fonts gravit-designer hk i-am-designer ifc insideoutside material-design-icons mode-777 onshape penpot photopea refero shp skribblio solidworks svg svgedit svgomg ynput-ayon-default""",
    "出行地图": """adventure-log air-trail apple-maps bikerouter boeing cessna dawarich diamond-aircraft embraer f4map flightradar24 foreflight komoot mapcomplete mapillary maptiler mapy met-office-uk nextbike oepnvmap osmand osmapp plane-finder shadowbroker traccar travelport wanderer waze""",
    "生活服务": """bar-assistant fasten-health google-fit healthlog mundraub nightscout openemr wger""",
    "电商购物": """2dehands albert-heijn carousell carrefour costco google-shopping gumroad idealo instacart kaufland kickstarter ko-fi lidl marktplaats newegg zabka""",
    "教育科研": """athenaeus bitesize class-dojo dictcc elabftw foldingathome italki kiwix linguacafe monkeytype moodle quizlet readthedocs sinf-uinsa the-io-foundation theodinproject trala w3c""",
    "新闻资讯": """associated-press bleacher-nation commafeed feedbase feedbin feedly feedlynx google-alerts morss newsblur nielseniq the-onion""",
    "阅读": """alexandrie archivedotorg biblioreads bindery booklore booko jelu koreader librum omnivore openreads readeck""",
    "搜索导航": """ecosia kagi mojeek seznam startpage""",
    "游戏": """5etools amazon-luna chess emulatorjs google-play-games itad lancommander modrinth oculus origin patzer playstation-plus polaris-gamestream red-dead-redemption-2 videogametrackarr wotdle xbox-game-pass""",
}

REVIEWED_CATEGORY = {slug: category for category, slugs in REVIEWED.items() for slug in slugs.split()}

ADDITIONAL_REVIEWED = {
    "AI": "buzz kubeflow willow",
    "开发编程": "automad cosign counter-analytics google-sites looker openchangelog release-argus rybbit",
    "网络工具": "chainguard freenom fusionauth kleopatra nordvpn parseable telegraf victoriametrics",
    "影音娱乐": "chevereto motioneye",
    "社交通讯": "chirpy hcl-verse pinkary pleroma quetre",
    "办公效率": "binner docsink formbricks note-mark notesnook radicale zapier",
    "体育运动": "endurain f1-dash fittrackee",
    "设计素材": "folder ",
    "系统工具": "cup devuan sailfishos toolz virtualbox",
    "出行地图": "gis-shapefile",
    "设备硬件": "hilook",
    "搜索导航": "fenrus google-lens linkace linkding neonlink searx searxng shaarli shiori",
    "阅读": "neodb ",
    "金融支付": "dkb ocular ycombinator",
    "游戏": "linuxgsm paper skribblio",
}
REVIEWED_CATEGORY.update({slug: category for category, slugs in ADDITIONAL_REVIEWED.items() for slug in slugs.split()})

# One reviewed rule set is shared with catalog maintenance and upstream sync.
_rules_path = Path(__file__).with_name("library-rules.json")
if _rules_path.is_file():
    _rules = json.loads(_rules_path.read_text(encoding="utf-8"))
    REVIEWED_CATEGORY.update({key.replace("/", "-"): category for key, category in _rules["categories"].items()})
    for source, target in [*_rules["merges"], *_rules["products"]]:
        category = REVIEWED_CATEGORY.get(target.replace("/", "-"))
        if category:
            REVIEWED_CATEGORY[source.replace("/", "-")] = category


def base_candidates(slug):
    """Wordmarks, colors and mascot variants represent the same product."""
    result = [slug]
    while True:
        base = re.sub(r"-(?:wordmark|light|dark|alt|flat|kawaii|black|white|logo|icon|full|symbol|fill|mono|svg|animated|type-\d+|english|japanese)$", "", result[-1])
        if base == result[-1]:
            return result
        result.append(base)


def upstream_category(labels, generic=False):
    categories = []
    for label in labels:
        key = normalized(label)
        if key in GENERIC_LABELS and not generic:
            continue
        category = LABEL_CATEGORY.get(key)
        if category and category not in categories:
            categories.append(category)
    # Prefer a specific function over a hosting/platform label.
    for category in ["AI", "开发编程", "网络工具", "影音娱乐", "浏览器", "办公效率", "社交通讯", "网盘存储", "金融支付", "游戏", "电商购物", "教育科研", "新闻资讯", "出行地图", "设计素材", "阅读", "生活服务", "搜索导航", "设备硬件", "云服务", "系统工具"]:
        if category in categories:
            return category
    return None




def infer_category(identifiers, labels, known):
    bases = list(dict.fromkeys(base for slug in identifiers if slug for base in base_candidates(slug)))
    for base in bases:
        if base in REVIEWED_CATEGORY:
            return REVIEWED_CATEGORY[base], f"reviewed product: {base}"
    specific = upstream_category(labels)
    if specific:
        return specific, "upstream taxonomy: " + ", ".join(sorted(set(labels)))
    for base in bases:
        if known.get(base) not in (None, "", "其他"):
            return known[base], f"same product variant: {base}"
    for base in bases:
        if base.startswith(("aws-", "azure-", "google-cloud-", "google-compute-")):
            return "云服务", "cloud platform family"
        if base.startswith(("avm-", "fritzbox-", "repeater-", "qnap-")):
            return "设备硬件", "hardware vendor/product family"
        if base.startswith("libreoffice-"):
            return "办公效率", "office suite product family"
        if base.endswith(("-linux", "-os")):
            return "系统工具", "operating system identifier"
    generic = upstream_category(labels, generic=True)
    if generic:
        return generic, "upstream utility/automation category"
    return None, "needs review"
