data "yandex_compute_image" "ubuntu_2404" {
  family = "ubuntu-2404-lts"
}
resource "yandex_vpc_address" "static-ip" {
  name = "edge-static-ip"
  external_ipv4_address {
    zone_id = "ru-central1-a"
  }
}

# 1. Создаем облачную сеть (VPC)
resource "yandex_vpc_network" "pro-network" {
  name = "edge-network"
}

# 2. Создаем подсеть в зоне "a"
resource "yandex_vpc_subnet" "pro-subnet-a" {
  name           = "edge-subnet-a"
  zone           = "ru-central1-a"
  network_id     = yandex_vpc_network.pro-network.id
  v4_cidr_blocks = ["192.168.10.0/24"]
}

resource "yandex_vpc_security_group" "edge-sg" {
  name       = "edge-sg"
  network_id = yandex_vpc_network.pro-network.id

  # Разрешаем входящий SSH (порт 22)
  ingress {
    protocol       = "TCP"
    description    = "Allow SSH"
    v4_cidr_blocks = ["0.0.0.0/0"] # В идеале тут должен быть твой домашний IP
    port           = 22
  }

  # Разрешаем HTTP (порт 80) для Nginx
  ingress {
    protocol       = "TCP"
    description    = "Allow HTTP"
    v4_cidr_blocks = ["0.0.0.0/0"]
    port           = 80
  }

  # Разрешаем HTTPS (порт 443) для Nginx и VLESS
  ingress {
    protocol       = "TCP"
    description    = "Allow HTTPS"
    v4_cidr_blocks = ["0.0.0.0/0"]
    port           = 443
  }
    # Разрешаем входящий трафик для API Kubernetes
  ingress {
    protocol       = "TCP"
    description    = "Allow K8s API"
    v4_cidr_blocks = ["0.0.0.0/0"] # В целях безопасности в будущем сюда лучше прописать свой домашний IP
    port           = 6443
  }
  # Разрешаем весь исходящий трафик (чтобы сервер мог качать обновления)
  egress {
    protocol       = "ANY"
    v4_cidr_blocks = ["0.0.0.0/0"]
    from_port      = 0
    to_port        = 65535
  }
}

# 3. Создаем Edge-Gateway (наш будущий прокси-фильтр)
resource "yandex_compute_instance" "edge-gateway" {
  name = "edge-gateway"
  platform_id = "standard-v3" # Актуальное поколение процессоров в 2026

  resources {
    cores  = 2
    memory = 2
    core_fraction = 20
  }

  scheduling_policy {
    preemptible = true # Сделать сервер ПРЕРЫВАЕМЫМ
  }
  boot_disk {
    initialize_params {
      image_id = data.yandex_compute_image.ubuntu_2404.id # Ubuntu 24.04 LTS в Yandex Cloud
      size     = 20
    }
  }

  network_interface {
    subnet_id = yandex_vpc_subnet.pro-subnet-a.id
    nat       = true # Даем публичный IP, так как это шлюз
    nat_ip_address = yandex_vpc_address.static-ip.external_ipv4_address[0].address
    security_group_ids = [yandex_vpc_security_group.edge-sg.id]

  }

  metadata = {
    ssh-keys = "ubuntu:${file("~/.ssh/id_ed25519.pub")}"
  }
}

output "gateway_public_ip" {
  description = "Статичный публичный IP нашего шлюза"
  value       = yandex_vpc_address.static-ip.external_ipv4_address[0].address
}

