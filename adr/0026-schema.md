# 25. Zarf Schema for 1.0

Date: 2024-06-07

## Status

Proposed

## Context

Zarf currently does not have explicit schema versions. Any schema changes are embedded into Zarf and can change with new versions. There are several examples of deprecated keys throughout Zarf's lifespan such as:

- `setVariable` deprecated in favor of `setVariables`
- `scripts` deprecated in favor of `actions`
- `required` soon to be deprecated in favor of `optional`.
- `group` deprecated in favor of `flavor`, however these work functionally different and cannot be automatically migrated
- `cosign` <- Austin to understand this one better

Zarf has not disabled any deprecated keys thus far. On create the user is always warned when using a deprecated field, however the field still exists in the schema and functions properly. Some of the deprecated keys can be migrated automatically as the old item is a subset or directly related to it's replacement. For example, setVariable is automatically migrated to a single item list of setVariables. This migration occurs on the zarf.yaml fed into the package during package create, however the original field is not deleted from the packaged zarf.yaml because the Zarf binary used in the airgap or delivery environment is not assumed to have the new schema fields that the deprecated key was migrated to.

There are two problems this ADR aims to solve
- Supporting deprecated features forever is complex and time consuming
- Dropping feature support can frustrate or confuse users

# Decisions

Zarf will introduce proper schema versions. A top level key, `apiVersion`, will be introduced to allow users to specify the schema. If `apiVersion` is not specified it will be assumed to be the 1.0 schema. In v1.0, deprecated features will be entirely deleted from Zarf. Any key that exists at the introduction of 1.0 will last the entirety of that schema lifetime. The features may be deprecated, but will not be removed until the next schema version.

The `zarf dev update-schema` command will be introduced to automatically update any deprecated fields in the users `zarf.yaml`, it will also add the apiVersion key and value. `zarf package create` will fail if the user has any deprecated keys and they will have a call to action to run `zarf dev update-schema`


BDD:
- *Given*: A user has a `zarf.yaml` with keys deprecated pre v1.0
  *when* the user runs `zarf package create`
  *then* they will receive an error and be told to run `zarf dev update-schema` if applicable or be told what to fix
- *Given*: A package is created with Zarf pre v1.0
  *and* that package has deprecated keys
  *when* the package is deployed with Zarf post v1.0
  *then* the package will be read without error, but the deprecated keys will be ignored.
- *Given*: A package is created with Zarf post 1.0
  *and* that package has deprecated keys
  *when* the deployed with Zarf version pre 1.0.
  *then* Zarf pre 1.0 will deploy the package without issues. If necessary this may mean doing translations in the before packaging the zarf.yaml similar to how the current deprecated migration steps work. We may do this with the `required` key depending on when it is deprecated in favor of `optional`.
