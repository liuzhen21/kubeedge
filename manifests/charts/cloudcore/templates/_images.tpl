{{- define "cloudcore.image" -}}
{{ include "common.images.image" (dict "imageRoot" .Values.cloudCore.image "global" .Values.global) }}
{{- end -}}


{{- define "controller.image" -}}
{{ include "common.images.image" (dict "imageRoot" .Values.controllerManager.image "global" .Values.global) }}
{{- end -}}


{{- define "admission.image" -}}
{{ include "common.images.image" (dict "imageRoot" .Values.admission.image "global" .Values.global) }}
{{- end -}}


{{- define "common.images.image" -}}
{{- $registryName := .global.imageRegistry -}}
{{- $repositoryName := .imageRoot.repository -}}
{{- $separator := ":" -}}
{{- $termination := .global.tag | toString -}}
{{- if and .imageRoot.registry (ne .imageRoot.registry "") }}
    {{- $registryName = .imageRoot.registry -}}
{{- end -}}
{{- if .imageRoot.tag }}
    {{- $termination = .imageRoot.tag | toString -}}
{{- end -}}
{{- if .imageRoot.digest }}
    {{- $separator = "@" -}}
    {{- $termination = .imageRoot.digest | toString -}}
{{- end -}}
{{- if $registryName }}
    {{- printf "%s/%s%s%s" $registryName $repositoryName $separator $termination -}}
{{- else -}}
    {{- printf "%s%s%s" $repositoryName $separator $termination -}}
{{- end -}}
{{- end -}}