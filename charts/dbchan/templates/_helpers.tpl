{{/*
Expand the name of the chart.
*/}}
{{- define "dbchan.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "dbchan.fullname" -}}
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
{{- define "dbchan.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "dbchan.labels" -}}
helm.sh/chart: {{ include "dbchan.chart" . }}
{{ include "dbchan.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "dbchan.selectorLabels" -}}
app.kubernetes.io/name: {{ include "dbchan.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "dbchan.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "dbchan.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the controller manager
*/}}
{{- define "dbchan.controllerManagerName" -}}
{{- printf "%s-controller-manager" (include "dbchan.fullname" .) }}
{{- end }}

{{/*
Create the name of the manager role
*/}}
{{- define "dbchan.managerRoleName" -}}
{{- printf "%s-manager-role" (include "dbchan.fullname" .) }}
{{- end }}

{{/*
Create the name of the manager role binding
*/}}
{{- define "dbchan.managerRoleBindingName" -}}
{{- printf "%s-manager-rolebinding" (include "dbchan.fullname" .) }}
{{- end }}

{{/*
Create the name of the leader election role
*/}}
{{- define "dbchan.leaderElectionRoleName" -}}
{{- printf "%s-leader-election-role" (include "dbchan.fullname" .) }}
{{- end }}

{{/*
Create the name of the leader election role binding
*/}}
{{- define "dbchan.leaderElectionRoleBindingName" -}}
{{- printf "%s-leader-election-rolebinding" (include "dbchan.fullname" .) }}
{{- end }}

{{/*
Create the name of the database editor role
*/}}
{{- define "dbchan.databaseEditorRoleName" -}}
{{- printf "%s-database-editor-role" (include "dbchan.fullname" .) }}
{{- end }}

{{/*
Create the name of the database viewer role
*/}}
{{- define "dbchan.databaseViewerRoleName" -}}
{{- printf "%s-database-viewer-role" (include "dbchan.fullname" .) }}
{{- end }}

{{/*
Create the name of the datastore editor role
*/}}
{{- define "dbchan.datastoreEditorRoleName" -}}
{{- printf "%s-datastore-editor-role" (include "dbchan.fullname" .) }}
{{- end }}

{{/*
Create the name of the datastore viewer role
*/}}
{{- define "dbchan.datastoreViewerRoleName" -}}
{{- printf "%s-datastore-viewer-role" (include "dbchan.fullname" .) }}
{{- end }}

{{/*
Create the name of the user editor role
*/}}
{{- define "dbchan.userEditorRoleName" -}}
{{- printf "%s-user-editor-role" (include "dbchan.fullname" .) }}
{{- end }}

{{/*
Create the name of the user viewer role
*/}}
{{- define "dbchan.userViewerRoleName" -}}
{{- printf "%s-user-viewer-role" (include "dbchan.fullname" .) }}
{{- end }}

{{/*
Create the name of the migration editor role
*/}}
{{- define "dbchan.migrationEditorRoleName" -}}
{{- printf "%s-migration-editor-role" (include "dbchan.fullname" .) }}
{{- end }}

{{/*
Create the name of the migration viewer role
*/}}
{{- define "dbchan.migrationViewerRoleName" -}}
{{- printf "%s-migration-viewer-role" (include "dbchan.fullname" .) }}
{{- end }}

{{/*
Create the image name for the controller manager
*/}}
{{- define "dbchan.controllerManagerImage" -}}
{{- $registry := .Values.controllerManager.manager.image.registry | default "" }}
{{- $repository := .Values.controllerManager.manager.image.repository }}
{{- $tag := .Values.controllerManager.manager.image.tag | default .Chart.AppVersion }}
{{- if $registry }}
{{- printf "%s/%s:%s" $registry $repository $tag }}
{{- else }}
{{- printf "%s:%s" $repository $tag }}
{{- end }}
{{- end }}

{{/*
Create the namespace for resources
*/}}
{{- define "dbchan.namespace" -}}
{{- .Release.Namespace }}
{{- end }}

{{/*
Create common annotations
*/}}
{{- define "dbchan.annotations" -}}
{{- if .Values.controllerManager.manager.annotations }}
{{- toYaml .Values.controllerManager.manager.annotations }}
{{- end }}
{{- end }}

{{/*
Create common node selector
*/}}
{{- define "dbchan.nodeSelector" -}}
{{- if .Values.controllerManager.manager.nodeSelector }}
{{- toYaml .Values.controllerManager.manager.nodeSelector }}
{{- end }}
{{- end }}

{{/*
Create common tolerations
*/}}
{{- define "dbchan.tolerations" -}}
{{- if .Values.controllerManager.manager.tolerations }}
{{- toYaml .Values.controllerManager.manager.tolerations }}
{{- end }}
{{- end }}

{{/*
Create common affinity
*/}}
{{- define "dbchan.affinity" -}}
{{- if .Values.controllerManager.manager.affinity }}
{{- toYaml .Values.controllerManager.manager.affinity }}
{{- end }}
{{- end }}
