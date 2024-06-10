# 25. Zarf Schema for 1

Date: 2024-06-07

## Status

Proposed

## Context

Zarf currently does not have explicit schema versions. Any schema changes are embedded into Zarf and can change with new versions. There are several examples of deprecated keys throughout Zarf's lifespan such as:

- `setVariable` deprecated in favor of `setVariables`
- `scripts` deprecated in favor of `actions`
- `required` soon to be deprecated in favor of `optional`.
- `group` deprecated in favor of `flavor`, however these work functionally different and cannot be automatically migrated
- `cosignKeyPath` deprecated in favor of specifying the key path at create time

Zarf has not disabled any deprecated keys thus far. On create the user is always warned when using a deprecated field, however the field still exists in the schema and functions properly. Some of the deprecated keys can be migrated automatically as the old item is a subset or directly related to it's replacement. For example, setVariable is automatically migrated to a single item list of setVariables. This migration occurs on the zarf.yaml fed into the package during package create, however the original field is not deleted from the packaged zarf.yaml because the Zarf binary used in the airgap or delivery environment is not assumed to have the new schema fields that the deprecated key was migrated to.

There are two problems this ADR aims to solve
- Supporting deprecated features forever is complex and time consuming
- Dropping feature support can frustrate or confuse users

# Decisions

Zarf will introduce proper schema versions. A top level key, `apiVersion`, will be introduced to allow users to specify the schema. If `apiVersion` is not specified we will break on deploy and instruct users to run `zarf dev update-schema`. Any package without this key will be assumed to use the alpha schema

In v1, deprecated features that have a direct migration path will cause an error on create, but will still be deployed if the package was created pre v1. If a feature does not have a direct automatic migration path (cosignKeyPath & groups) they will fail on deploy. This will exist until the alpha schema is entirely removed from Zarf, which will happen one year after v1 is released.

The existing types which comprise the Zarf schema will be moved to types/alpha and be renamed to alpha. Once v1 is released this package will not be touched again. A copy of these types will be created in a package called v1

Any key that exists at the introduction of v1 will last the entirety of that schema lifetime. The features may be deprecated, but will not be removed until the next schema version.

The `zarf dev update-schema` command will be introduced to automatically update deprecated fields in the users `zarf.yaml` where possible. It will also add the apiVersion key and set it to v1. `zarf package create` will fail if the user has deprecated keys with a list of the deprecated keys and a call to action to run `zarf dev update-schema`.


## BDD scenarios
### 1 create with deprecated keys
- *Given*: A user has a `zarf.yaml` with keys deprecated pre v1
- *when* the user runs `zarf package create`
- *then* they will receive an error and be told to run `zarf dev update-schema` or how to migrate off cosign key paths or how to use flavors over groups depending on the error

### pre 1 auto migrate create -> 1 deploy
- *Given*: A package is created with Zarf pre v1
- *and* that package has deprecated keys that can be automatically migrated (required, scripts, & set variables)
- *when* the package is deployed with Zarf v1
- *then* the keys will be automatically migrated & the package will be deployed without error.

### pre 1 create feature removal -> 1 deploy
- *Given*: A package is created with Zarf pre v1
- *and* that package has deprecated keys that cannot be automatically migrated (groups, cosignKeyPath)
- *when* the package is deployed with Zarf v1
- *then* then deploy of that package will fail and the user will be instructed to update their package

### 1 create -> pre 1 deploy
- *Given*: A package is created with Zarf v1
- *and* that package uses keys that did not exist in pre v1
- *when* the package is deployed with Zarf pre v1
- *then* Zarf pre 1 will deploy the package without issues. If there is an automatic migration to a previous field that then will take place. If there is not an automatic migration path, then the user will be warned they are deploying a package that has schema fields that are unrecognized in the current Zarf version.

# Make a plan

- Document that the v1 package is what we use in all our code
- Potentially we support previous version of the schema
- If a new feature is created it will not exist in the old schema if deployed by an old package, we should log when the schema changes.
- If you are creating on v1 you must specify api version. This can also be done automatically through zarf dev update schema
