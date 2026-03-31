terraform {
  required_providers {
    yandex = {
      source = "yandex-cloud/yandex"
      version = ">= 0.100.0" 
    }
  }
}

provider "yandex" {
  service_account_key_file = "key.json"
  cloud_id                 = "b1g0r7ub552edbd9lasm"
  folder_id                = "b1gagv1eqakakb2jdhjq"
  zone                     = "ru-central1-a"
}