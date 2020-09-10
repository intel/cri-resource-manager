pipeline {
    agent {
        label "cri-rm"
    }

    environment {
        IMAGE_REPO = "cloud-native-image-registry.westus.cloudapp.azure.com"
    }

    stages {
        stage('Build and push images') {
            steps {
                withDockerRegistry([credentialsId: "${env.DOCKER_REGISTRY}", url: "https://${env.IMAGE_REPO}"]) {
                    sh "make images-push IMAGE_REPO=${env.IMAGE_REPO}"
                }
            }
        }
    }
}
