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

There are two problems this ADR aims to solve:
- Supporting deprecated features forever is complex and time consuming
- Dropping feature support can frustrate or confuse users

## Decision

Zarf will begin having proper schema versions. A top level key, `apiVersion`, will be introduced to allow users to specify the schema. At the release of v1 the only valid user input for `apiVersion` will be v1. Zarf will not allow users to build at the pre v1 version. `zarf package create` will fail if the user has deprecated keys or if `apiVersion` is missing and the user will be instructed to run the new `zarf dev update-schema` command. `zarf dev update-schema` will automatically migrate deprecated fields in the users `zarf.yaml` where possible. It will also add the apiVersion key and set it to v1.

The existing go types which comprise the Zarf schema will be moved to types/alpha and will never change. An updated copy of these types without the deprecated fields will be created in a package types/v1 and all schema changes will happen on these objects. Any place in the code that is using a struct included in the Zarf schema will change from `types.structName` to `v1.structName`.

All deprecated features will cause an error on create. Deprecated features with a direct migration path will still be deployed if the package was created v1, as migrations will add the non deprecated fields. If a feature does not have a direct automatic migration path (cosignKeyPath & groups) the package will fail on deploy. This will happen until the alpha schema is entirely removed from Zarf, which will happen one year after v1 is released.

Any key that exists at the introduction of v1 will last the entirety of that schema lifetime. The features may be deprecated, but will not be removed until the next schema version.

### BDD scenarios
#### v1 create with deprecated keys
- *Given* Zarf version is v1
- *and* the `zarf.yaml` has no apiVersion or deprecated keys
- *when* the user runs `zarf package create`
- *then* they will receive an error and be told to run `zarf dev update-schema` or how to migrate off cosign key paths or how to use flavors over groups depending on the error

#### pre v1 create -> v1 deploy
- *Given*: A package is created with Zarf pre v1
- *and* that package has deprecated keys that can be automatically migrated (required, scripts, & set variables)
- *when* the package is deployed with Zarf v1
- *then* the keys will be automatically migrated & the package will be deployed without error.

#### pre v1 create feature removal -> v1 deploy
- *Given*: A package is created with Zarf pre v1
- *and* that package has deprecated keys that cannot be automatically migrated (groups, cosignKeyPath)
- *when* the package is deployed with Zarf v1
- *then* then deploy of that package will fail and the user will be instructed to update their package

#### v1 create -> pre v1 deploy
- *Given*: A package is created with Zarf v1
- *and* that package uses keys that did not exist in pre v1
- *when* the package is deployed with Zarf pre v1
- *then* Zarf pre 1 will deploy the package without issues. If there is an automatic migration to a previous field that then will take place. If the field is unrecognized by the schema, then the user will be warned they are deploying a package that has features that do not exist in the current version of Zarf.

## Consequences
- Users of deprecated group or cosignKeyPath might be frustrated if their packages, created prev1, error out on Zarf v1, however this is likely preferable to unexpected behavior occurring in the cluster.
- We will have to have two different schema types which will be mostly be duplicate code. However the original type should never change, which mitigates much of the issue.
- Users may be frustrated that they have to run `zarf dev update-schema` to edit their `zarf.yaml` to remove the deprecated fields and add `apiVersion`.
