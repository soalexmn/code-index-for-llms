variable "environment" {
  description = "Deployment environment name (e.g. staging, production)"
  type        = string
}

variable "region" {
  description = "AWS region for all resources"
  type        = string
  default     = "us-east-1"
}

variable "db_password" {
  description = "Master password for the RDS PostgreSQL instance"
  type        = string
  sensitive   = true
}

variable "instance_count" {
  description = "Desired number of API instances in the autoscaling group"
  type        = number
  default     = 2
}

variable "allowed_cidrs" {
  description = "CIDR blocks allowed to reach the web security group"
  type        = list(string)
  default     = ["0.0.0.0/0"]
}
