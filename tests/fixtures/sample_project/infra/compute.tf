resource "aws_launch_template" "api" {
  name_prefix   = "${var.environment}-api-"
  image_id      = "ami-0c55b159cbfafe1f0"
  instance_type = "t3.small"

  vpc_security_group_ids = [aws_security_group.web.id]

  user_data = base64encode(<<-EOF
    #!/bin/bash
    yum update -y
    systemctl start api-server
  EOF
  )

  tag_specifications {
    resource_type = "instance"
    tags = { Name = "${var.environment}-api", Environment = var.environment }
  }
}

resource "aws_autoscaling_group" "api" {
  name                = "${var.environment}-api-asg"
  min_size            = 1
  max_size            = var.instance_count * 2
  desired_capacity    = var.instance_count
  vpc_zone_identifier = [aws_subnet.public_a.id, aws_subnet.public_b.id]

  launch_template {
    id      = aws_launch_template.api.id
    version = "$Latest"
  }

  health_check_type         = "ELB"
  health_check_grace_period = 300

  tag {
    key                 = "Environment"
    value               = var.environment
    propagate_at_launch = true
  }
}

resource "aws_instance" "bastion" {
  ami                    = "ami-0c55b159cbfafe1f0"
  instance_type          = "t3.micro"
  subnet_id              = aws_subnet.public_a.id
  vpc_security_group_ids = [aws_security_group.web.id]

  tags = { Name = "${var.environment}-bastion", Role = "bastion" }
}
