{{/*
Expand the name of the chart.
*/}}
{{- define "draforge.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "draforge.fullname" -}}
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

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "draforge.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "draforge.labels" -}}
helm.sh/chart: {{ include "draforge.chart" . }}
{{ include "draforge.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
project: draforge
{{- end }}

{{/*
Selector labels
*/}}
{{- define "draforge.selectorLabels" -}}
app.kubernetes.io/name: {{ include "draforge.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Build an image reference from registry, repository, tag/digest and appVersion.
Digest takes precedence; an empty tag defaults to Chart.appVersion.
*/}}
{{- define "draforge.image" -}}
{{- $registry := trimSuffix "/" (required "global.imageRegistry is required" .registry) -}}
{{- $repository := trimPrefix "/" (required "image.repository is required" .repository) -}}
{{- $image := printf "%s/%s" $registry $repository -}}
{{- if .digest -}}
{{- printf "%s@%s" $image .digest -}}
{{- else -}}
{{- $tag := default .appVersion .tag -}}
{{- printf "%s:%s" $image (required "image.tag or Chart.appVersion is required" $tag) -}}
{{- end -}}
{{- end }}

{{/*
Restricted egress rules shared by DRAForge workloads.
By default, workloads may resolve DNS and reach Kubernetes-style API server
ports without allowing every protocol and port to every destination.
*/}}
{{- define "draforge.restrictedEgress" -}}
{{- if .Values.networkPolicies.allowAllEgress }}
- {}
{{- else }}
- to:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: kube-system
  ports:
    - protocol: UDP
      port: 53
    - protocol: TCP
      port: 53
- ports:
    - protocol: TCP
      port: 443
    - protocol: TCP
      port: 6443
{{- end }}
{{- end }}
