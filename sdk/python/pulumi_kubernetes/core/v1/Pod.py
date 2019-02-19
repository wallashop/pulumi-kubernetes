import pulumi
import pulumi.runtime
from typing import Optional

from ... import tables

class Pod(pulumi.CustomResource):
    """
    Pod is a collection of containers that can run on a host. This resource is created by clients
    and scheduled onto hosts.
    """
    def __init__(self, __name__, __opts__=None, metadata=None, spec=None, status=None):
        if not __name__:
            raise TypeError('Missing resource name argument (for URN creation)')
        if not isinstance(__name__, str):
            raise TypeError('Expected resource name to be a string')
        if __opts__ and not isinstance(__opts__, pulumi.ResourceOptions):
            raise TypeError('Expected resource options to be a ResourceOptions instance')

        __props__ = dict()

        __props__['apiVersion'] = 'v1'
        __props__['kind'] = 'Pod'
        __props__['metadata'] = metadata
        __props__['spec'] = spec
        __props__['status'] = status

        super(Pod, self).__init__(
            "kubernetes:core/v1:Pod",
            __name__,
            __props__,
            __opts__)

    def get(self, __name__: str,
            identifier: pulumi.Input[str],
            __opts__: Optional[pulumi.ResourceOptions] = None
            ):
        """
        Get the state of an existing `Pod` resource, as identified by `identifier`.
        Typically this identifier  is of the form <namespace>/<name>; if <namespace> is omitted, then (per
        Kubernetes convention) the identifier becomes default/<name>.
       
        Pulumi will keep track of this resource using `name` as the Pulumi ID.
    
        :param __name__: _Unique_ name used to register this resource with Pulumi.
        :param identifier: An ID for the Kubernetes resource to retrieve. Takes the form
        <namespace>/<name> or <name>.
        :param __opts__: Uniquely specifies a CustomResource to select.
        """
        return Pod(__name__, __opts__, )

    def translate_output_property(self, prop: str) -> str:
        return tables._CASING_FORWARD_TABLE.get(prop) or prop

    def translate_input_property(self, prop: str) -> str:
        return tables._CASING_BACKWARD_TABLE.get(prop) or prop
