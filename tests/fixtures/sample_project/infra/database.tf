resource "aws_db_subnet_group" "main" {
  name       = "${var.environment}-db-subnet-group"
  subnet_ids = [aws_subnet.public_a.id, aws_subnet.public_b.id]

  tags = { Name = "${var.environment}-db-subnet-group", Environment = var.environment }
}

resource "aws_db_instance" "primary" {
  identifier        = "${var.environment}-postgres"
  engine            = "postgres"
  engine_version    = "15.4"
  instance_class    = "db.t3.micro"
  allocated_storage = 20
  storage_type      = "gp3"

  db_name  = "appdb"
  username = "appuser"
  password = var.db_password

  db_subnet_group_name   = aws_db_subnet_group.main.name
  vpc_security_group_ids = [aws_security_group.web.id]

  skip_final_snapshot     = false
  final_snapshot_identifier = "${var.environment}-final-snapshot"
  deletion_protection     = true
  backup_retention_period = 7

  tags = { Name = "${var.environment}-postgres", Environment = var.environment }
}
