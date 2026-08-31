dependency "vpc" {
  config_path = "../vpc"
}

# Marked from inputs, not locals: locals are evaluated before dependency outputs exist.
inputs = {
  policy = templatefile(mark_as_read("policy.json.tftpl"), {
    vpc_id = dependency.vpc.outputs.vpc_id
  })
}
