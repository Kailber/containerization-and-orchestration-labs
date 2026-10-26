"""Run official KRR 1.30.0 with read-only discovery of CloudNativePG Clusters.

Only discovery and JSON export are extended. PromQL, simple strategy,
rounding, minimum recommendations and table rendering remain upstream KRR.
Run inside the pinned upstream Docker image, whose source lives in /app.
"""

import asyncio
import logging
import os
from pathlib import Path
import sys
from typing import Literal

sys.path.insert(0, "/app")

from kubernetes.client import V1LabelSelector
from robusta_krr.core.integrations.kubernetes import ClusterLoader
from robusta_krr.core.models.allocations import ResourceAllocations
from robusta_krr.core.models.objects import K8sObjectData
from robusta_krr.core.runner import Runner
from robusta_krr.utils.object_like_dict import ObjectLikeDict

logger = logging.getLogger("krr")


class CNPGObject(K8sObjectData):
    kind: Literal["Cluster"] = "Cluster"

    @property
    def selector(self):
        return V1LabelSelector(match_labels={"cnpg.io/cluster": self.name})


def discover_cnpg(loader):
    namespaces = loader.namespaces
    if namespaces == "*":
        namespaces = [ns.metadata.name for ns in loader.core.list_namespace().items]
    objects = []
    for namespace in namespaces:
        clusters = loader.custom_objects.list_namespaced_custom_object(
            group="postgresql.cnpg.io", version="v1", plural="clusters",
            namespace=namespace,
        )["items"]
        for cluster in clusters:
            metadata = cluster["metadata"]
            pods = loader.core.list_namespaced_pod(
                namespace=namespace,
                label_selector=f"cnpg.io/cluster={metadata['name']}",
            ).items
            instances = [
                pod for pod in pods
                if pod.status.phase == "Running"
                and pod.metadata.deletion_timestamp is None
                and any(
                    owner.uid == metadata["uid"]
                    and owner.kind == "Cluster"
                    and owner.api_version == "postgresql.cnpg.io/v1"
                    for owner in pod.metadata.owner_references or []
                )
            ]
            if not instances:
                raise RuntimeError(f"No running instances for CNPG {namespace}/{metadata['name']}")
            containers = [
                container for pod in instances for container in pod.spec.containers
                if container.name == "postgres"
            ]
            if len(containers) != len(instances):
                raise RuntimeError("Expected one postgres container per CNPG instance")
            allocations = ResourceAllocations.from_container(containers[0])
            if any(ResourceAllocations.from_container(c) != allocations for c in containers[1:]):
                raise RuntimeError("CNPG instances have different resources; scan after rollout completes")
            obj = CNPGObject(
                cluster=loader.cluster, namespace=namespace,
                name=metadata["name"], container="postgres", allocations=allocations,
                hpa=None, labels=metadata.get("labels", {}),
                annotations=metadata.get("annotations", {}),
            )
            obj._api_resource = ObjectLikeDict(cluster)
            objects.append(obj)
            logger.info("Discovered CNPG Cluster %s/%s (%d instances)", namespace, obj.name, len(instances))
    return objects


original_discovery = ClusterLoader.list_scannable_objects


async def list_with_cnpg(self):
    objects = await original_discovery(self)
    objects.extend(await asyncio.to_thread(discover_cnpg, self))
    return objects


original_process = Runner._process_result


def process_with_json(self, result):
    output = os.environ.get("KRR_JSON_OUTPUT")
    if output:
        result.errors = self.errors
        Path(output).write_text(result.json(indent=2) + "\n")
    return original_process(self, result)


if __name__ == "__main__":
    ClusterLoader.list_scannable_objects = list_with_cnpg
    Runner._process_result = process_with_json
    from robusta_krr.main import run
    run()
