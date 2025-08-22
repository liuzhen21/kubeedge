{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "cloudcore.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}


{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
*/}}
{{- define "cloudcore.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Generate certificates for kubeedge cloudstream server
*/}}
{{- define "cloudcore.gen-certs" -}}
{{- $caCrt := "" }}
{{- $caKey := "" }}
{{- $ns := .Release.Namespace }}
{{- $rootCASecret := (lookup "v1" "Secret" $ns "edgewize-root-ca" ) -}}
{{- if $rootCASecret -}}
{{- $caCrt = index $rootCASecret.data "rootCA.crt" -}}
{{- $caKey = index $rootCASecret.data "rootCA.key" -}}
{{- else -}}
{{- fail "edgewize-root-ca not found" -}}
{{- end -}}
{{- $rootCA := "" -}}
{{- if and $caCrt $caKey -}}
{{- $rootCA = buildCustomCert $caCrt $caKey -}}
{{- else -}}
{{- fail "certificate or private key in secret edgewize-root-ca invalid" -}}
{{- end -}}
{{- $ips := .Values.cloudCore.modules.cloudHub.advertiseAddress }}
{{- $newIps := list }}
{{- range $ips }}
{{- $newIps = append $newIps . }}
{{- end }}
{{- $altName_cloudcore1 := printf "cloudcore.%s" .Release.Namespace }}
{{- $altName_cloudcore2 := printf "cloudcore.%s.svc" .Release.Namespace }}
{{- $cn_cloudcore := printf "Kubeedge Cloudcore" }}
{{- $cert_cloudcore := genSignedCert $cn_cloudcore $newIps (list $altName_cloudcore1 $altName_cloudcore2) 36500 $rootCA -}}
tlsCA.crt: {{ $rootCA.Cert | b64enc }}
tlsCA.key: {{ $rootCA.Key | b64enc }}
server.crt: {{ $cert_cloudcore.Cert | b64enc }}
server.key: {{ $cert_cloudcore.Key | b64enc }}
{{- end -}}

{{- define "admission.gen-certs" -}}
{{- $caCrt := "" }}
{{- $caKey := "" }}
{{- $ns := .Release.Namespace }}
{{- $rootCASecret := (lookup "v1" "Secret" $ns "edgewize-root-ca" ) -}}
{{- if $rootCASecret -}}
{{- $caCrt = index $rootCASecret.data "rootCA.crt" -}}
{{- $caKey = index $rootCASecret.data "rootCA.key" -}}
{{- else -}}
{{- fail "edgewize-root-ca not found" -}}
{{- end -}}
{{- $rootCA := "" -}}
{{- if and $caCrt $caKey -}}
{{- $rootCA = buildCustomCert $caCrt $caKey -}}
{{- else -}}
{{- fail "certificate or private key in secret edgewize-root-ca invalid" -}}
{{- end -}}
{{- $ips := .Values.cloudCore.modules.cloudHub.advertiseAddress }}
{{- $newIps := list }}
{{- range $ips }}
{{- $newIps = append $newIps . }}
{{- end }}
{{- $altName1 := printf "kubeedge-admission-service.%s" .Release.Namespace }}
{{- $altName2 := printf "kubeedge-admission-service.%s.svc" .Release.Namespace }}
{{- $cn := printf "Kubeedge Admission" }}
{{- $cert := genSignedCert $cn $newIps (list $altName1 $altName2) 36500 $rootCA -}}
ca.crt: {{ $rootCA.Cert | b64enc }}
ca.key: {{ $rootCA.Key | b64enc }}
tls.crt: {{ $cert.Cert | b64enc }}
tls.key: {{ $cert.Key | b64enc }}
{{- end -}}

