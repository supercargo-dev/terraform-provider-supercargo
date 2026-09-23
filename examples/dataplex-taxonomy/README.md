# Enterprise Dataplex Taxonomy Example

This example demonstrates how to use the `dataplex_taxonomy` module to provision Google Cloud Data Catalog / Dataplex taxonomies and policy tags for enterprise data mesh governance with Supercargo.

## Architecture

The module provisions:
- A Data Catalog Taxonomy (`Corporate Sensitivity`) with `FINE_GRAINED_ACCESS_CONTROL` enabled.
- Four base sensitivity tiers (`PUBLIC`, `INTERNAL`, `CONFIDENTIAL`, `RESTRICTED`).
- Customer-parameterized sub-categories parented under `RESTRICTED` (e.g. `FINANCIAL`, `PERSONAL`, `SECURITY`, `HEALTH`).

The resulting policy tag map (`output.policy_tags`) provides zero-config URN resolution in Supercargo Hub and BigQuery DWH synchronization (`sc dwh sync`).
