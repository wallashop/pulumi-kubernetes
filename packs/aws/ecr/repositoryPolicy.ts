// *** WARNING: this file was generated by the Lumi Terraform Bridge (TFGEN) Tool. ***
// *** Do not edit by hand unless you're certain you know what you are doing! ***

import * as lumi from "@lumi/lumi";

export class RepositoryPolicy extends lumi.NamedResource implements RepositoryPolicyArgs {
    public readonly policy: string;
    public readonly registryId?: string;
    public readonly repository: string;

    constructor(name: string, args: RepositoryPolicyArgs) {
        super(name);
        this.policy = args.policy;
        this.registryId = args.registryId;
        this.repository = args.repository;
    }
}

export interface RepositoryPolicyArgs {
    readonly policy: string;
    readonly registryId?: string;
    readonly repository: string;
}
