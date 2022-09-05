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
                script {
                    withDockerRegistry([credentialsId: "${env.DOCKER_REGISTRY}", url: "https://${env.IMAGE_REPO}"]) {
                        if (env.BRANCH_NAME == 'master') {
                            sh "make images-push IMAGE_REPO=${env.IMAGE_REPO} IMAGE_VERSION=devel Q="
                        } else {
                            sh "make images-push IMAGE_REPO=${env.IMAGE_REPO} Q="
                        }
                    }
                }
            }
        }
    }
}
