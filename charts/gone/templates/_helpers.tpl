{{/* Chart name, truncated to fit a label. */}}
{{- define "gone.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/* Fully qualified name: the release name, plus the chart name unless the release already contains it. */}}
{{- define "gone.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{- define "gone.selectorLabels" -}}
app.kubernetes.io/name: {{ include "gone.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "gone.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{ include "gone.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/* The image reference: a digest wins over a tag; the tag defaults to appVersion. */}}
{{- define "gone.image" -}}
{{- if .Values.image.digest }}
{{- printf "%s@%s" .Values.image.repository .Values.image.digest }}
{{- else }}
{{- printf "%s:%s" .Values.image.repository (default .Chart.AppVersion .Values.image.tag) }}
{{- end }}
{{- end }}

{{/* The claim holding /data. */}}
{{- define "gone.claimName" -}}
{{- default (include "gone.fullname" .) .Values.persistence.existingClaim }}
{{- end }}

{{/* Providers that verify a signed token. Plain-header providers aren't supported on Kubernetes. */}}
{{- define "gone.signedProviders" -}}
cloudflare google-iap pomerium teleport oidc
{{- end }}

{{/* True when sign-in uses a signed provider (which fetches keys over HTTPS). */}}
{{- define "gone.signedAuth" -}}
{{- if has .Values.auth.provider (splitList " " (include "gone.signedProviders" .)) }}true{{ end }}
{{- end }}

{{/*
gone.validate fails the render for settings the chart refuses, each with the
reason. It renders nothing.
*/}}
{{- define "gone.validate" -}}
{{- $doc := "https://github.com/haukened/gone/blob/main/docs/deploy/kubernetes.md#sign-in" }}
{{- $p := .Values.auth.provider }}
{{- if has $p (list "authelia" "authentik" "tailscale" "custom") }}
{{- fail (printf "auth.provider=%s: plain-header sign-in isn't supported on Kubernetes, because any pod in the cluster could send the header Gone trusts. Use a signed provider (cloudflare, google-iap, pomerium, teleport or oidc); %s explains why and what to use instead" $p $doc) }}
{{- end }}
{{- if not (or (eq $p "none") (include "gone.signedAuth" .)) }}
{{- fail (printf "auth.provider=%s: unknown provider; use none, cloudflare, google-iap, pomerium, teleport or oidc" $p) }}
{{- end }}
{{- range .Values.extraEnv }}
{{- if hasPrefix "GONE_AUTH" .name }}
{{- fail (printf "extraEnv %s: set sign-in through auth, not extraEnv; plain-header providers aren't supported on Kubernetes (%s)" .name $doc) }}
{{- end }}
{{- if has .name (list "GONE_ADDR" "GONE_DATA_DIR" "GONE_METRICS_ADDR" "GONE_METRICS_TOKEN" "GONE_BRAND_LOGO") }}
{{- fail (printf "extraEnv %s: the chart sets this itself; use the matching value instead" .name) }}
{{- end }}
{{- end }}
{{- if and (hasKey .Values "replicaCount") (ne (toString .Values.replicaCount) "1") }}
{{- fail "replicaCount: Gone runs one replica. Its SQLite database and blob directory allow one writer, and rate limits live in memory; a second pod would corrupt data or lose secrets. See https://github.com/haukened/gone/blob/main/docs/deploy/kubernetes.md#one-replica" }}
{{- end }}
{{- if and .Values.metrics.enabled (not .Values.metrics.token.existingSecret) }}
{{- fail "metrics.enabled needs metrics.token.existingSecret: Gone serves metrics only behind a bearer token" }}
{{- end }}
{{- if and .Values.metrics.serviceMonitor.enabled (not .Values.metrics.enabled) }}
{{- fail "metrics.serviceMonitor.enabled needs metrics.enabled" }}
{{- end }}
{{- if and .Values.brand.logo.existingConfigMap (not .Values.brand.logo.key) }}
{{- fail "brand.logo.key: name the file in brand.logo.existingConfigMap, e.g. logo.svg" }}
{{- end }}
{{- if and .Values.httpRoute.enabled (not .Values.httpRoute.parentRefs) }}
{{- fail "httpRoute.enabled needs httpRoute.parentRefs: the Gateway to attach to" }}
{{- end }}
{{- end }}

{{/* settings pairs each GONE_* variable the chart passes through with its value. */}}
{{- define "gone.settings" -}}
{{- $c := .Values.config }}
{{- $p := .Values.policy }}
{{- $b := .Values.brand }}
{{- $a := .Values.auth }}
{{- $s := list
  (list "GONE_MAX_BYTES" $c.maxBytes) (list "GONE_INLINE_MAX_BYTES" $c.inlineMaxBytes)
  (list "GONE_TTL_OPTIONS" $c.ttlOptions) (list "GONE_CLAIM_LEASE" $c.claimLease)
  (list "GONE_RATE_CREATE" $c.rateCreate) (list "GONE_RATE_READ" $c.rateRead)
  (list "GONE_RATE_BURST" $c.rateBurst) (list "GONE_TRUSTED_PROXIES" $c.trustedProxies)
  (list "GONE_REQUIRE_PASSPHRASE" $p.requirePassphrase) (list "GONE_MAX_FILES" $p.maxFiles)
  (list "GONE_BRAND_NAME" $b.name) (list "GONE_BRAND_ACCENT" $b.accent)
  (list "GONE_CONTACT_URL" $b.contactURL) (list "GONE_IMPRINT_URL" $b.imprintURL)
  (list "GONE_PRIVACY_URL" $b.privacyURL) }}
{{- if ne $a.provider "none" }}
{{- $s = concat $s (list
  (list "GONE_AUTH" $a.provider) (list "GONE_AUTH_TEAM" $a.team)
  (list "GONE_AUTH_AUDIENCE" $a.audience) (list "GONE_AUTH_ISSUER" $a.issuer)
  (list "GONE_AUTH_HOST" $a.host) (list "GONE_AUTH_JWKS_URL" $a.jwksURL)
  (list "GONE_AUTH_ALG" $a.alg) (list "GONE_AUTH_HEADER" $a.header)
  (list "GONE_AUTH_SIGNIN_URL" $a.signInURL)) }}
{{- end }}
{{- toJson $s }}
{{- end }}

{{/*
gone.env renders the set settings as container env entries: null, "" and []
are left out so Gone keeps its default; lists join with commas.
*/}}
{{- define "gone.env" -}}
{{- $env := list }}
{{- range (include "gone.settings" . | fromJsonArray) }}
{{- $name := index . 0 }}
{{- $v := index . 1 }}
{{- /* Nested ifs, not and/or: Helm before 3.10 evaluates every operand. */}}
{{- $set := true }}
{{- if kindIs "invalid" $v }}
{{- $set = false }}
{{- else if kindIs "string" $v }}
{{- if eq $v "" }}{{ $set = false }}{{ end }}
{{- else if kindIs "slice" $v }}
{{- $v = join "," (compact $v) }}
{{- if eq $v "" }}{{ $set = false }}{{ end }}
{{- else if kindIs "float64" $v }}
{{- $v = int64 $v }}
{{- end }}
{{- if $set }}
{{- $env = append $env (dict "name" $name "value" (toString $v)) }}
{{- end }}
{{- end }}
{{- if $env }}
{{- toYaml $env }}
{{- end }}
{{- end }}
