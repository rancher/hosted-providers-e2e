With every rancher release, a new operator version may or may not be released. _This document provides a reference to what tests should be run for both the cases._

For release testing, we test against development Rancher builds; for e.g. to test 2.11, we use `latest/devel/2.11` rancher version (`v2.11-head` image tag). These builds are similar to RC builds, as both utilize the `dev-v2.x` branch of the Rancher chart repository, where application charts for operators are prepared for upcoming release.

Before validation, always sync with developers to ensure the desired versions have been merged into the charts and Rancher projects.

`P0` suite must be run to test every new rancher release, preferably from Rancher Dashboard. This suite tests provisioning and importing clusters, k8s upgrade of the control plane and nodes, and CRUD operations of the nodegroup/nodepool.

If the operator contains any update, the [entire test suite](https://app.qase.io/project/HP) must be tested except `K8sChartSupportUpgrade*`; unless the operator also adds support for a new K8s version.

If the operator does not contain any update, running `P0*` from Rancher Dashboard should suffice for prime-only rancher release (for e.g 2.8, 2.9; at the time of writing this doc), along with some ad-hoc UI testing for a non-GA rancher minor version (for e.g. v2.11).
