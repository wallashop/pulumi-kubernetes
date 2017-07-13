// *** WARNING: this file was generated by the Lumi Terraform Bridge (TFGEN) Tool. ***
// *** Do not edit by hand unless you're certain you know what you are doing! ***

import * as lumi from "@lumi/lumi";

export class TemplateDeployment extends lumi.NamedResource implements TemplateDeploymentArgs {
    public readonly deploymentMode: string;
    public readonly _name: string;
    public readonly outputs?: {[key: string]: any};
    public readonly parameters?: {[key: string]: any};
    public readonly resourceGroupName: string;
    public readonly templateBody?: string;

    constructor(name: string, args: TemplateDeploymentArgs) {
        super(name);
        this.deploymentMode = args.deploymentMode;
        this._name = args._name;
        this.outputs = args.outputs;
        this.parameters = args.parameters;
        this.resourceGroupName = args.resourceGroupName;
        this.templateBody = args.templateBody;
    }
}

export interface TemplateDeploymentArgs {
    readonly deploymentMode: string;
    readonly _name: string;
    readonly outputs?: {[key: string]: any};
    readonly parameters?: {[key: string]: any};
    readonly resourceGroupName: string;
    readonly templateBody?: string;
}
