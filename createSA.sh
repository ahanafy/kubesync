#!/bin/bash

# replace this with your own gcp project id and the name of the service account
# that will be created.
PROJECT_ID=gcpproject
NEW_SA_NAME=kubesync-sa

printf "Begin script\n"

# create service account
SA="${NEW_SA_NAME}@${PROJECT_ID}.iam.gserviceaccount.com"
if gcloud iam service-accounts describe $SA --project $PROJECT_ID --no-user-output-enabled 2>/dev/null; then printf "%s found. \n\nSkipping creation...\n" $SA; else gcloud iam service-accounts create $NEW_SA_NAME --project $PROJECT_ID; fi

# grant access to cloud API
## declare all required roles
declare -a ROLES=("roles/secretmanager.admin")

## now loop through all roles
printf "\nSetting Roles...\n"
for i in "${ROLES[@]}"
do
   echo "$i"
   gcloud projects add-iam-policy-binding --role="$i" $PROJECT_ID --member "serviceAccount:$SA" --no-user-output-enabled
done

printf "\nChecking Roles...\n"
gcloud projects get-iam-policy $PROJECT_ID  \
--flatten="bindings[].members" \
--format='table(bindings.role)' \
--filter="bindings.members:$SA"

printf "\nDiscovering Old Keys\n"
OLDKEYS=()
while IFS="," read -r KEYID
do
    printf "Old existing keys: %s\n\n" "$KEYID"
	OLDKEYS+=("$KEYID")
done < <(gcloud iam service-accounts keys list --project=$PROJECT_ID --iam-account="$SA" --managed-by=user --format 'csv[no-heading](KEY_ID)')

# create service account keyfile
gcloud iam service-accounts keys create creds.json --project $PROJECT_ID --iam-account $SA

for OLDKEY in "${OLDKEYS[@]}"
do
     printf "Deleting: %s\n" "$OLDKEY"
	 gcloud iam service-accounts keys delete "$OLDKEY" --project $PROJECT_ID --iam-account $SA --quiet
done

printf "\nListing Registered Keys...\n"
gcloud iam service-accounts keys list --project $PROJECT_ID --iam-account $SA --managed-by user