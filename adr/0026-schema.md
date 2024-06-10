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

Zarf will introduce proper schema versions. A top level key, `apiVersion`, will be introduced to allow users to specify the schema. `zarf package create` will fail at v1 if the user has deprecated keys or is missing `apiVersion` and the user will be instructed to run the new `zarf dev update-schema` command. `zarf dev update-schema` will automatically update deprecated fields in the users `zarf.yaml` where possible. It will also add the apiVersion key and set it to v1.

The existing go types which comprise the Zarf schema will be moved to types/alpha. Once v1 is released this types/alpha will not be edited. A copy of these types will be created in a package types/v1 and this is where active development will happen.

In v1, deprecated features that have a direct migration path will cause an error on create, but will still be deployed if the package was created pre v1. Pre v1 packages are packages that do not have the apiVersion key set. In code when these packages are read they will be automatically migrated to the type v1 package and all functions will accept v1 types. If a feature does not have a direct automatic migration path (cosignKeyPath & groups) the package will fail on deploy. This will happen until the alpha schema is entirely removed from Zarf, which will happen one year after v1 is released.

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

## Option to discuss

At create time Zarf will package both a `zarf.yaml` and a `zarfv1.yaml`, if a `zarfv1.yaml` exists Zarf will use that. If a `zarfv1.yaml` does not exist, then Zarf will know that the package was prev1 and use the regular `zarf.yaml`. This will allow new features or schema changes added post v1 to be migrated to the old Zarf schema so the new features can be deployed preV1

## Consequences

- Users of deprecated group do not have a direct replacement, though this will happen regardless, and we've been warning users of several months
- We will have to have two different schema types which will be mostly copies of one another. However the original schema should never change
-
