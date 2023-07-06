#!/usr/bin/env bash

# script base directory
BASE_DIR=$(cd $(dirname "$0")/$(dirname "$(readlink $0)") && pwd -P)

# ---------- Main functions ----------

. ${BASE_DIR}/utils.sh

# check_catalogsource check if the given catalogsource is available for selected packagemanifest and channel
function check_catalogsource() {
    local catalog_source=$1
    local catalog_namespace=$2
    local package_manifest=$3
    local operator_namespace=$4
    local channel=$5
    local return_value=0
    local result=$(${OC} get packagemanifest -n $operator_namespace -o yaml | ${YQ} eval '.items[] | select(.status.catalogSource == "'${catalog_source}'" and .status.catalogSourceNamespace == "'${catalog_namespace}'" and .status.packageName == "'${package_manifest}'" and .status.channels[].name == "'${channel}'") | .status.catalogSource')
    if [[ -z "$result" || "$result" == "null" ]]; then
        return_value=1
    fi
    echo "$return_value"
}


function get_catalogsource() {
    local package_manifest=$1
    local operator_namespace=$2
    local channel=$3
    local count=0
    local catalog_source=""
    local catalog_namespace=""

    local result=$(${OC} get packagemanifest -n $operator_namespace -o yaml | ${YQ} eval '.items[] | select(.status.packageName == "'${package_manifest}'" and .status.channels[].name == "'${channel}'") | {"name": .status.catalogSource, "namespace": .status.catalogSourceNamespace}')
    
    local total_count=$(wc -w <<< "$result")
    count=$((total_count / 4))
    if [[ count -eq 1 ]]; then
        catalog_source=$(cut -f2 -d " " <<< $result)
        catalog_namespace=$(cut -f4 -d " " <<< $result)
    fi
    echo "$count $catalog_source $catalog_namespace"
}

OC="oc"
YQ="yq"
CatalogSource="opencloud-operators-v4-1"
CatalogNamespace="openshift-marketplace"
PackageManifest="ibm-common-service-operator"
OperatorNamespace="default"
Channel="v4.0"
# result=$(check_catalogsource opencloud-operators-v4-1 openshift-marketplace ibm-common-service-operator default v4.0)
result=$(check_catalogsource $CatalogSource $CatalogNamespace $PackageManifest $OperatorNamespace $Channel)
if [[ $result == "1" ]]; then
    warning "CatalogSource $CatalogSource from $CatalogNamespace namespace is not available for $PackageManifest in $OperatorNamespace namespace"
    result=$(get_catalogsource $PackageManifest $OperatorNamespace $Channel)
    IFS=" " read -r count catalog catalog_ns <<< "$result"
    # if count is greater then 1
    if [[ $count -gt 1 ]]; then
        error "Multiple CatalogSource are available for $PackageManifest in $OperatorNamespace namespace, please specify the correct CatalogSource name and namespace"
    elif [[ $count -eq 0 ]]; then
        error "No CatalogSource is available for $PackageManifest in $OperatorNamespace namespace"
    else
        catalog_source="$catalog"
        catalog_namespace="$catalog_ns"
        info "CatalogSource $catalog_source from $catalog_namespace namespace is available for $PackageManifest in $OperatorNamespace namespace"
    fi
else
    info "CatalogSource $CatalogSource from $CatalogNamespace namespace is available for $PackageManifest in $OperatorNamespace namespace"
fi