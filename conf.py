# Configuration file for the Sphinx documentation builder.
#
# This file only contains a selection of the most common options. For a full
# list see the documentation:
# https://www.sphinx-doc.org/en/master/usage/configuration.html

# -- Path setup --------------------------------------------------------------

# If extensions (or modules to document with autodoc) are in another directory,
# add these directories to sys.path here. If the directory is relative to the
# documentation root, use os.path.abspath to make it absolute, like shown here.
#
# import os
# import sys
# sys.path.insert(0, os.path.abspath('.'))
from docutils import nodes
from os.path import isdir, isfile, join, basename, dirname
from os import makedirs, getenv
from shutil import copyfile

# -- Project information -----------------------------------------------------

project = 'CRI Resource Manager'
copyright = '2020, various'
author = 'various'


##############################################################################
#
# This section determines the behavior of links to local items in .md files.
#
#  if useGitHubURL == True:
#
#     links to local files and directories will be turned into github URLs
#     using either the baseBranch defined here or using the commit SHA.
#
#  if useGitHubURL == False:
#
#     local files will be moved to the website directory structure when built
#     local directories will still be links to github URLs
#
#  if built with GitHub workflows:
#
#     the GitHub URLs will use the commit SHA (GITHUB_SHA environment variable
#     is defined by GitHub workflows) to link to the specific commit.
#
##############################################################################

baseBranch = "master"
useGitHubURL = True
commitSHA = getenv('GITHUB_SHA')
githubBaseURL = "https://github.com/intelkevinputnam/cri-resource-manager/"
githubFileURL = githubBaseURL + "blob/"
githubDirURL = githubBaseURL + "tree/"
if commitSHA:
    githubFileURL = githubFileURL + commitSHA + "/"
    githubDirURL = githubDirURL + commitSHA + "/"
else:
    githubFileURL = githubFileURL + baseBranch + "/"
    githubDirURL = githubDirURL + baseBranch + "/"

# -- General configuration ---------------------------------------------------

# Add any Sphinx extension module names here, as strings. They can be
# extensions coming with Sphinx (named 'sphinx.ext.*') or your custom
# ones.
extensions = ['recommonmark','sphinx_markdown_tables']
source_suffix = {'.rst': 'restructuredtext','.md': 'markdown'}


# Add any paths that contain templates here, relative to this directory.
templates_path = ['_templates']

# List of patterns, relative to source directory, that match files and
# directories to ignore when looking for source files.
# This pattern also affects html_static_path and html_extra_path.
exclude_patterns = ['_build', 'Thumbs.db', '.DS_Store','_work']


# -- Options for HTML output -------------------------------------------------

# The theme to use for HTML and HTML Help pages.  See the documentation for
# a list of builtin themes.
#
html_theme = 'sphinx_rtd_theme'

# Add any paths that contain custom static files (such as style sheets) here,
# relative to this directory. They are copied after the builtin static files,
# so a file named "default.css" will overwrite the builtin "default.css".
#html_static_path = ['_static']

def setup(app):
    app.connect('doctree-resolved',fixLocalMDAnchors)
    app.connect('missing-reference',fixRSTLinkInMD)

###############################################################################
#
#  This section defines callbacks that make markdown specific tweaks to
#  either:
#
#  1. Fix something that recommonmark does wrong.
#  2. Provide support for .md files that are written as READMEs in a GitHub
#     repo.
#
#  Only use these changes if using the extension ``recommonmark``.
#
###############################################################################


# Callback registerd with 'missing-reference'.
def fixRSTLinkInMD(app, env, node, contnode):
    refTarget = node.get('reftarget')
    filePath = refTarget.lstrip("/")
    if '.rst' in refTarget and "://" not in refTarget:
    # This occurs when a .rst file is referenced from a .md file
    # Currently unable to check if file exists as no file
    # context is provided and links are relative.
    #
    # Example: [Application examples](examples/readme.rst)
    #
        contnode['refuri'] = contnode['refuri'].replace('.rst','.html')
        contnode['internal'] = "True"
        return contnode
    else:
    # This occurs when a file is referenced for download from an .md file.
    # Construct a list of them and short-circuit the warning. The files
    # are moved later (need file location context). To avoid warnings,
    # write .md files, make the links absolute. This only marks them fixed
    # if it can verify that they exist.
    #
    # Example: [Makefile](/Makefile)
    #
        if isfile(filePath) or isdir(filePath):
            return contnode


def normalizePath(docPath,uriPath):
    if uriPath == "":
        return uriPath
    if "#" in uriPath:
    # Strip out anchors
        uriPath = uriPath.split("#")[0]
    if uriPath.startswith("/"):
    # It's an absolute path
        return uriPath.lstrip("/") #path to file from project directory
    else:
    # It's a relative path
        docDir = dirname(docPath)
        return join(docDir,uriPath) #path to file from referencing file


# Callback registerd with 'doctree-resolved'.
def fixLocalMDAnchors(app, doctree, docname):
    for node in doctree.traverse(nodes.reference):
        uri = node.get('refuri')
        filePath = normalizePath(docname,uri)
        if isfile(filePath):
        # Only do this if the file exists.
        #
        # TODO: Pop a warning if the file doesn't exist.
        #
            if '.md' in uri and '://' not in uri:
            # Make sure .md file links that weren't caught are converted.
            # These occur when creating an explicit link to an .md file
            # from an .rst file. By default these are not validated by Sphinx
            # or recommonmark. Only toctree references are validated. recommonmark
            # also fails to convert links to local Markdown files that include
            # anchors. This fixes that as well.
            #
            # Only include this code if .md files are being converted to html
            #
            # Example: `Google Cloud Engine <gce.md>`__
            #          [configuration options](autotest.md#configuration-options)
            #
                node['refuri'] = node['refuri'].replace('.md','.html')
            else:
            # Handle the case where markdown is referencing local files in the repo
            #
            # Example: [Makefile](/Makefile)
            #
                if useGitHubURL:
                # Replace references to local files with links to the GitHub repo
                #
                    newURI = githubFileURL + filePath
                    print("new url: ", newURI)
                    node['refuri']=newURI
                else:
                # If there are links to local files other than .md (.rst files are caught
                # when warnings are fired), move the files into the Sphinx project, so
                # they can be accessed.
                    newFileDir = join(app.outdir,dirname(filePath)) # where to move the file in Sphinx output.
                    newFilePath = join(app.outdir,filePath)
                    newURI = uri # if the path is relative no need to change it.
                    if uri.startswith("/"):
                    # It's an absolute path. Need to make it relative.
                        uri = uri.lstrip("/")
                        docDirDepth = len(docname.split("/")) - 1
                        newURI = "../"*docDirDepth + uri
                    if not isdir(newFileDir):
                        makedirs(newFileDir)
                    copyfile(filePath,newFilePath)
                    node['refuri'] = newURI
        elif "#" not in uri: # ignore anchors
        # turn links to directories into links to the repo
            if isdir(filePath):
                newURI = githubDirURL + filePath
                node['refuri']=newURI
